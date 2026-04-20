package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/config"
)

// buildJob holds the state of a single firmware build run.
type buildJob struct {
	ID  string
	mu  sync.Mutex
	log []string
	done bool
	err  string
}

func (j *buildJob) appendLine(line string) {
	j.mu.Lock()
	j.log = append(j.log, line)
	j.mu.Unlock()
}

func (j *buildJob) finish(errMsg string) {
	j.mu.Lock()
	j.err = errMsg
	j.done = true
	j.mu.Unlock()
}

// snapshotFrom returns log lines starting at cursor position, new cursor, and done/err state.
func (j *buildJob) snapshotFrom(cursor int) (lines []string, next int, done bool, errMsg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if cursor < len(j.log) {
		lines = make([]string, len(j.log)-cursor)
		copy(lines, j.log[cursor:])
		next = len(j.log)
	} else {
		next = cursor
	}
	return lines, next, j.done, j.err
}

var (
	buildMu     sync.Mutex
	activeBuild *buildJob
)

func buildRouter(cfg *config.Config) func(chi.Router) {
	return func(r chi.Router) {
		r.Post("/", startBuild(cfg))
		r.Get("/status", getBuildStatus())
		r.Get("/{id}/logs", streamBuildLogs())
	}
}

func startBuild(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		srcDir := os.Getenv("FIRMWARE_SRC_DIR")
		if srcDir == "" {
			http.Error(w, "FIRMWARE_SRC_DIR not configured (mount firmware source and set env var)", http.StatusServiceUnavailable)
			return
		}

		buildMu.Lock()
		if activeBuild != nil && !activeBuild.done {
			buildMu.Unlock()
			http.Error(w, "build already in progress", http.StatusConflict)
			return
		}
		job := &buildJob{ID: fmt.Sprintf("%d", time.Now().UnixMilli())}
		activeBuild = job
		buildMu.Unlock()

		go runBuild(job, srcDir, cfg.DataDir)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"id": job.ID})
	}
}

func getBuildStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		buildMu.Lock()
		job := activeBuild
		buildMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if job == nil {
			json.NewEncoder(w).Encode(map[string]any{"running": false})
			return
		}
		_, _, done, errMsg := job.snapshotFrom(0)
		json.NewEncoder(w).Encode(map[string]any{
			"id":      job.ID,
			"running": !done,
			"done":    done,
			"error":   errMsg,
		})
	}
}

func streamBuildLogs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		buildMu.Lock()
		job := activeBuild
		buildMu.Unlock()

		if job == nil || job.ID != id {
			http.Error(w, "build job not found", http.StatusNotFound)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // disable Nginx/Traefik buffering

		cursor := 0
		for {
			lines, next, done, errMsg := job.snapshotFrom(cursor)
			for _, line := range lines {
				fmt.Fprintf(w, "data: %s\n\n", line)
			}
			cursor = next
			if len(lines) > 0 {
				flusher.Flush()
			}

			// All lines drained and job is finished → send terminal event.
			if done && cursor == next {
				if errMsg != "" {
					fmt.Fprintf(w, "event: fail\ndata: %s\n\n", errMsg)
				} else {
					fmt.Fprintf(w, "event: done\ndata: ok\n\n")
				}
				flusher.Flush()
				return
			}

			select {
			case <-r.Context().Done():
				return
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
}

// runBuild compiles firmware using the matter-fw-builder Docker image.
//
// Strategy:
//  1. If the image exists, mount the updated main/ sources and run an
//     incremental idf.py build inside the existing image — this only
//     recompiles changed files and avoids the slow (30-60 min) SDK rebuild.
//  2. If the image does NOT exist, run a full docker build to create it.
//     On first run the Matter SDK clone + compile takes 30-60 min.
//
// The watcher auto-registers any .bin dropped into <incoming>/.
func runBuild(job *buildJob, srcDir, dataDir string) {
	incomingDir := filepath.Join(dataDir, "firmware", "incoming")
	if err := os.MkdirAll(incomingDir, 0o755); err != nil {
		job.appendLine("ERROR mkdir incoming: " + err.Error())
		job.finish("mkdir failed")
		return
	}

	// Check whether the builder image already exists.
	checkOut, _ := exec.Command("docker", "images", "-q", "matter-fw-builder").Output()
	imageExists := len(checkOut) > 0

	mainDir := filepath.Join(srcDir, "main")

	if imageExists {
		// Incremental build: mount updated main/ over /firmware/main and rebuild.
		// chip_gn (Matter SDK) is cached inside the image; only app code recompiles.
		job.appendLine("=== matter-fw-builder image found — incremental build ===")
		script := `set -e
. /opt/esp/idf/export.sh 2>/dev/null
. /opt/esp-matter/export.sh 2>/dev/null
cd /firmware
idf.py build 2>&1
VER=$(date +v%Y%m%d-%H%M%S)
cp build/matter_hub_firmware.bin /output/matter_hub_firmware_${VER}.bin
echo "Copied: matter_hub_firmware_${VER}.bin"
`
		if !execStep(job, "docker", "run", "--rm",
			"-v", incomingDir+":/output",
			"-v", mainDir+":/firmware/main:ro",
			"matter-fw-builder", "bash", "-c", script) {
			job.finish("incremental build failed")
			return
		}
	} else {
		// Full build: create the builder image from Dockerfile.build.
		dockerfilePath := filepath.Join(srcDir, "Dockerfile.build")
		job.appendLine("=== Step 1/2: docker build (30-60 min on first run — Matter SDK clone) ===")
		if !execStep(job, "docker", "build", "-t", "matter-fw-builder", "-f", dockerfilePath, srcDir) {
			job.finish("docker build failed")
			return
		}
		job.appendLine("=== Step 2/2: extract binary ===")
		if !execStep(job, "docker", "run", "--rm", "-v", incomingDir+":/output", "matter-fw-builder") {
			job.finish("docker run failed")
			return
		}
	}

	job.appendLine("=== Build complete — watcher will auto-register the binary within 5 s ===")
	job.finish("")
}

// execStep runs a command, streaming its combined stdout+stderr into job.log.
func execStep(job *buildJob, name string, args ...string) bool {
	job.appendLine(fmt.Sprintf("$ %s %v", name, args))

	cmd := exec.Command(name, args...)

	pr, pw, err := os.Pipe()
	if err != nil {
		job.appendLine("ERROR pipe: " + err.Error())
		return false
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		job.appendLine("ERROR start: " + err.Error())
		return false
	}
	pw.Close()

	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		job.appendLine(scanner.Text())
	}
	pr.Close()

	if err := cmd.Wait(); err != nil {
		job.appendLine("FAILED: " + err.Error())
		return false
	}
	return true
}
