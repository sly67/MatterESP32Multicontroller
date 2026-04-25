package esphome

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/library"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
)

// JobStatus is the state of a compile job.
type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobDone      JobStatus = "done"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

// JobConfig is stored as config_json and contains everything needed to re-compile.
type JobConfig struct {
	Board         string            `json:"board"`
	DeviceName    string            `json:"device_name"`
	DeviceID      string            `json:"device_id"`
	WiFiSSID      string            `json:"wifi_ssid"`
	WiFiPassword  string            `json:"wifi_password"`
	HAIntegration bool              `json:"ha_integration"`
	APIKey        string            `json:"api_key"`
	OTAPassword   string            `json:"ota_password"`
	Components    []ComponentConfig `json:"components"`
}

// Event is an SSE event broadcast to job subscribers.
type Event struct {
	Type     string `json:"type"`            // "log","status","done","position"
	Data     string `json:"data,omitempty"`  // log line / status / queue position
	OK       bool   `json:"ok,omitempty"`    // for "done"
	ErrorMsg string `json:"error,omitempty"` // for "done" when !OK
}

type queueJob struct {
	id     string
	config JobConfig
}

// Queue manages ESPHome compile jobs with a single worker goroutine.
type Queue struct {
	mu      sync.Mutex
	pending []*queueJob
	active  *queueJob
	cancel  context.CancelFunc

	subsMu sync.RWMutex
	subs   map[string][]chan Event

	database *db.Database
	sidecar  *Client
	dataDir  string
	notify   chan struct{}
}

// DataDir returns the data directory used by this Queue.
func (q *Queue) DataDir() string { return q.dataDir }

// NewQueue creates a Queue and starts the background worker and purge goroutines.
func NewQueue(database *db.Database, sidecar *Client, dataDir string) *Queue {
	q := &Queue{
		subs:     make(map[string][]chan Event),
		database: database,
		sidecar:  sidecar,
		dataDir:  dataDir,
		notify:   make(chan struct{}, 1),
	}
	os.MkdirAll(filepath.Join(dataDir, "esphome-builds"), 0755) //nolint:errcheck
	go q.worker()
	go q.purgeBinaries()
	return q
}

// Enqueue adds a new compile job and returns the job ID.
func (q *Queue) Enqueue(cfg JobConfig) (string, error) {
	id, err := randomHexID(6)
	if err != nil {
		return "", err
	}
	cfgJSON, _ := json.Marshal(cfg)
	if err := q.database.CreateJob(db.ESPHomeJob{
		ID:         id,
		DeviceName: cfg.DeviceName,
		ConfigJSON: string(cfgJSON),
		Status:     string(JobPending),
	}); err != nil {
		return "", fmt.Errorf("persist job: %w", err)
	}
	jb := &queueJob{id: id, config: cfg}
	q.mu.Lock()
	q.pending = append(q.pending, jb)
	pos := len(q.pending)
	q.mu.Unlock()

	q.broadcastPos(id, pos)
	select {
	case q.notify <- struct{}{}:
	default:
	}
	return id, nil
}

// Cancel cancels a pending or running job.
func (q *Queue) Cancel(id string) error {
	q.mu.Lock()
	for i, jb := range q.pending {
		if jb.id == id {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			q.mu.Unlock()
			q.database.UpdateJobStatus(id, string(JobCancelled), "", "cancelled") //nolint:errcheck
			q.broadcast(id, Event{Type: "status", Data: "cancelled"})
			q.broadcast(id, Event{Type: "done", ErrorMsg: "cancelled"})
			q.closeSubscribers(id)
			return nil
		}
	}
	if q.active != nil && q.active.id == id {
		if q.cancel != nil {
			q.cancel()
		}
		q.mu.Unlock()
		return nil
	}
	q.mu.Unlock()
	return fmt.Errorf("job %s not found or already finished", id)
}

// Subscribe returns a read-only event channel and a cleanup func.
// It replays existing log from DB before attaching to live updates.
func (q *Queue) Subscribe(id string) (<-chan Event, func(), error) {
	job, err := q.database.GetJob(id)
	if err != nil {
		return nil, nil, fmt.Errorf("job not found: %w", err)
	}
	ch := make(chan Event, 128)

	// Replay accumulated log
	if job.Log != "" {
		for _, line := range strings.Split(job.Log, "\n") {
			if line != "" {
				ch <- Event{Type: "log", Data: line}
			}
		}
	}
	ch <- Event{Type: "status", Data: job.Status}

	switch JobStatus(job.Status) {
	case JobDone:
		ch <- Event{Type: "done", OK: true}
		close(ch)
		return ch, func() {}, nil
	case JobFailed:
		ch <- Event{Type: "done", ErrorMsg: job.Error}
		close(ch)
		return ch, func() {}, nil
	case JobCancelled:
		ch <- Event{Type: "status", Data: "cancelled"}
		ch <- Event{Type: "done", ErrorMsg: "cancelled"}
		close(ch)
		return ch, func() {}, nil
	}

	q.subsMu.Lock()
	// Re-read status while holding the write lock to prevent a race where
	// closeSubscribers runs between our initial status check and registration.
	recheckJob, recheckErr := q.database.GetJob(id)
	if recheckErr == nil {
		switch JobStatus(recheckJob.Status) {
		case JobDone:
			q.subsMu.Unlock()
			ch <- Event{Type: "done", OK: true}
			close(ch)
			return ch, func() {}, nil
		case JobFailed:
			q.subsMu.Unlock()
			ch <- Event{Type: "done", ErrorMsg: recheckJob.Error}
			close(ch)
			return ch, func() {}, nil
		case JobCancelled:
			q.subsMu.Unlock()
			ch <- Event{Type: "done", ErrorMsg: "cancelled"}
			close(ch)
			return ch, func() {}, nil
		}
	}
	q.subs[id] = append(q.subs[id], ch)
	q.subsMu.Unlock()

	cleanup := func() {
		q.subsMu.Lock()
		defer q.subsMu.Unlock()
		list := q.subs[id]
		for i, c := range list {
			if c == ch {
				q.subs[id] = append(list[:i], list[i+1:]...)
				return
			}
		}
	}
	return ch, cleanup, nil
}

func (q *Queue) broadcast(id string, ev Event) {
	q.subsMu.RLock()
	list := make([]chan Event, len(q.subs[id]))
	copy(list, q.subs[id])
	q.subsMu.RUnlock()
	for _, ch := range list {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (q *Queue) broadcastPos(id string, pos int) {
	q.broadcast(id, Event{Type: "position", Data: fmt.Sprintf("%d", pos)})
}

func (q *Queue) closeSubscribers(id string) {
	q.subsMu.Lock()
	list := q.subs[id]
	delete(q.subs, id)
	q.subsMu.Unlock()
	for _, ch := range list {
		close(ch)
	}
}

func (q *Queue) worker() {
	for {
		<-q.notify

		for {
			q.mu.Lock()
			if len(q.pending) == 0 {
				q.mu.Unlock()
				break
			}
			jb := q.pending[0]
			q.pending = q.pending[1:]
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			q.active = jb
			q.cancel = cancel
			q.mu.Unlock()

			q.runJob(ctx, jb)
			cancel()

			q.mu.Lock()
			q.active = nil
			q.cancel = nil
			q.mu.Unlock()
		}
	}
}

// jobLogWriter implements io.Writer to broadcast log lines and persist them.
type jobLogWriter struct {
	jobID string
	queue *Queue
}

func (w *jobLogWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\r\n"), "\n") {
		if line != "" {
			line = strings.TrimRight(line, "\r")
			w.queue.database.AppendJobLog(w.jobID, line) //nolint:errcheck
			w.queue.broadcast(w.jobID, Event{Type: "log", Data: line})
		}
	}
	return len(p), nil
}

func (q *Queue) runJob(ctx context.Context, jb *queueJob) {
	q.database.UpdateJobStatus(jb.id, string(JobRunning), "", "") //nolint:errcheck
	q.broadcast(jb.id, Event{Type: "status", Data: string(JobRunning)})

	cfg := jb.config

	// Assemble YAML
	mods, err := library.LoadModules()
	if err != nil {
		q.failJob(jb, fmt.Errorf("load modules: %w", err))
		return
	}
	modMap := make(map[string]*yamldef.Module, len(mods))
	for _, m := range mods {
		modMap[m.ID] = m
	}

	// Build effect-param defaults per compatible module type, then fill any
	// missing params on each component so unset placeholders don't survive
	// into the generated YAML as literal "{PARAM}" strings.
	if effs, err2 := library.LoadEffects(); err2 == nil {
		defaults := make(map[string]map[string]string)
		for _, e := range effs {
			for _, compat := range e.CompatibleWith {
				if defaults[compat] == nil {
					defaults[compat] = make(map[string]string)
				}
				for _, p := range e.Params {
					if p.Default != nil {
						defaults[compat][p.ID] = fmt.Sprintf("%v", p.Default)
					}
				}
			}
		}
		for i := range cfg.Components {
			comp := &cfg.Components[i]
			if d, ok := defaults[comp.Type]; ok {
				if comp.EffectParams == nil {
					comp.EffectParams = make(map[string]string)
				}
				for k, v := range d {
					if _, exists := comp.EffectParams[k]; !exists {
						comp.EffectParams[k] = v
					}
				}
			}
		}
	}

	// Fill io config/float/select defaults for any missing pins.
	// Prevents unresolved {KEY} placeholders in generated YAML when caller omits optional pins.
	fillIODefaults(cfg.Components, modMap)

	yamlStr, err := Assemble(Config{
		Board:         cfg.Board,
		DeviceName:    cfg.DeviceName,
		DeviceID:      cfg.DeviceID,
		WiFiSSID:      cfg.WiFiSSID,
		WiFiPassword:  cfg.WiFiPassword,
		HAIntegration: cfg.HAIntegration,
		APIKey:        cfg.APIKey,
		OTAPassword:   cfg.OTAPassword,
		Components:    cfg.Components,
	}, modMap)
	if err != nil {
		q.failJob(jb, fmt.Errorf("assemble YAML: %w", err))
		return
	}

	// Write YAML + SDK header wrappers to shared cache volume
	devSlug := slug(cfg.DeviceName)
	devDir := filepath.Join(q.dataDir, "esphome-cache", devSlug)
	if err := os.MkdirAll(devDir, 0755); err != nil {
		q.failJob(jb, fmt.Errorf("create dev dir: %w", err))
		return
	}
	if err := os.WriteFile(filepath.Join(devDir, "config.yaml"), []byte(yamlStr), 0644); err != nil {
		q.failJob(jb, fmt.Errorf("write config: %w", err))
		return
	}
	for rel, content := range map[string]string{
		"driver/ledc.h": "#pragma once\n#include <driver/ledc.h>\n",
	} {
		wrapPath := filepath.Join(devDir, rel)
		os.MkdirAll(filepath.Dir(wrapPath), 0755)     //nolint:errcheck
		os.WriteFile(wrapPath, []byte(content), 0644) //nolint:errcheck
	}

	// Compile via sidecar
	logWriter := &jobLogWriter{jobID: jb.id, queue: q}
	if err := q.sidecar.Compile(ctx, devSlug, logWriter); err != nil {
		if ctx.Err() != nil {
			q.database.UpdateJobStatus(jb.id, string(JobCancelled), "", "cancelled") //nolint:errcheck
			q.broadcast(jb.id, Event{Type: "status", Data: "cancelled"})
			q.broadcast(jb.id, Event{Type: "done", ErrorMsg: "cancelled"})
			q.closeSubscribers(jb.id)
			return
		}
		q.failJob(jb, err)
		return
	}

	// Read compiled binary from shared volume
	binSrc := filepath.Join(q.dataDir, "esphome-cache", devSlug,
		".esphome", "build", devSlug, ".pioenvs", devSlug, "firmware.factory.bin")
	binData, err := os.ReadFile(binSrc)
	if err != nil {
		q.failJob(jb, fmt.Errorf("read firmware binary: %w", err))
		return
	}

	// Save binary to persistent store
	binDest := filepath.Join(q.dataDir, "esphome-builds", jb.id+".bin")
	if err := os.WriteFile(binDest, binData, 0644); err != nil {
		q.failJob(jb, fmt.Errorf("save firmware: %w", err))
		return
	}

	// Register/update device in DB (idempotent on re-compile)
	espCfgJSON, _ := json.Marshal(struct {
		Board         string            `json:"board"`
		HAIntegration bool              `json:"ha_integration"`
		OTAPassword   string            `json:"ota_password"`
		Components    []ComponentConfig `json:"components"`
	}{cfg.Board, cfg.HAIntegration, cfg.OTAPassword, cfg.Components})
	q.database.CreateDevice(db.Device{ //nolint:errcheck // may already exist on re-compile
		ID:            cfg.DeviceID,
		Name:          cfg.DeviceName,
		FirmwareType:  "esphome",
		ESPHomeConfig: string(espCfgJSON),
		ESPHomeAPIKey: cfg.APIKey,
		PSK:           []byte{},
	})

	q.database.UpdateJobDone(jb.id, binDest, cfg.DeviceID) //nolint:errcheck
	q.broadcast(jb.id, Event{Type: "done", OK: true})
	q.closeSubscribers(jb.id)
}

func (q *Queue) failJob(jb *queueJob, err error) {
	q.database.UpdateJobStatus(jb.id, string(JobFailed), "", err.Error()) //nolint:errcheck
	q.broadcast(jb.id, Event{Type: "done", ErrorMsg: err.Error()})
	q.closeSubscribers(jb.id)
}

func (q *Queue) purgeBinaries() {
	for {
		time.Sleep(24 * time.Hour)
		cutoff := time.Now().Add(-7 * 24 * time.Hour)
		q.database.DeleteOldJobs(cutoff) //nolint:errcheck
		entries, _ := os.ReadDir(filepath.Join(q.dataDir, "esphome-builds"))
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err == nil && info.ModTime().Before(cutoff) {
				os.Remove(filepath.Join(q.dataDir, "esphome-builds", e.Name())) //nolint:errcheck
			}
		}
	}
}

func randomHexID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Ensure jobLogWriter implements io.Writer (compile-time check).
var _ io.Writer = (*jobLogWriter)(nil)

// fillIODefaults fills missing config/float/select pins from module IO defaults.
// This prevents unresolved {KEY} placeholders from surviving into generated YAML.
func fillIODefaults(components []ComponentConfig, modMap map[string]*yamldef.Module) {
	for i := range components {
		comp := &components[i]
		mod, ok := modMap[comp.Type]
		if !ok {
			continue
		}
		for _, pin := range mod.IO {
			if pin.Default == "" {
				continue
			}
			switch pin.Type {
			case "config", "float", "select":
				if comp.Pins == nil {
					comp.Pins = make(map[string]string)
				}
				if _, exists := comp.Pins[pin.ID]; !exists {
					comp.Pins[pin.ID] = pin.Default
				}
			}
		}
	}
}
