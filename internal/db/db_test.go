package db_test

import (
	"testing"
	"time"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_CreatesTables(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	expected := []string{"devices", "templates", "modules", "effects", "firmware", "flash_log", "ota_log"}
	for _, tbl := range expected {
		row := database.DB.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl)
		var name string
		require.NoError(t, row.Scan(&name), "table %q missing", tbl)
		assert.Equal(t, tbl, name)
	}
}

func TestDevice_CreateAndGet(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	// Insert a template first so the FK constraint on devices.template_id is satisfied.
	_, err = database.DB.Exec(
		`INSERT INTO templates (id, name, board, yaml_body) VALUES (?, ?, ?, ?)`,
		"firefly-hub-v1", "Firefly Hub v1", "esp32-c3", "{}",
	)
	require.NoError(t, err)

	dev := db.Device{
		ID:         "esp-test01",
		Name:       "1/Bedroom",
		TemplateID: "firefly-hub-v1",
		PSK:        []byte("testpsk"),
	}
	require.NoError(t, database.CreateDevice(dev))

	got, err := database.GetDevice("esp-test01")
	require.NoError(t, err)
	assert.Equal(t, "1/Bedroom", got.Name)
	assert.Equal(t, []byte("testpsk"), got.PSK)
}

func TestDevice_UpdateMatterCreds(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	_, err = database.DB.Exec(
		`INSERT INTO templates (id, name, board, yaml_body) VALUES (?, ?, ?, ?)`,
		"tpl-1", "T1", "esp32-c3", "id: tpl-1",
	)
	require.NoError(t, err)
	require.NoError(t, database.CreateDevice(db.Device{
		ID: "dev-1", Name: "1/Bedroom", TemplateID: "tpl-1", PSK: make([]byte, 32),
	}))

	require.NoError(t, database.UpdateDeviceMatterCreds("dev-1", 3840, 20202021))

	got, err := database.GetDevice("dev-1")
	require.NoError(t, err)
	assert.Equal(t, uint16(3840), got.MatterDiscrim)
	assert.Equal(t, uint32(20202021), got.MatterPasscode)
}

func TestDevice_CreateESPHome(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	require.NoError(t, database.CreateDevice(db.Device{
		ID:            "dev-esph",
		Name:          "Kitchen Sensor",
		FirmwareType:  "esphome",
		ESPHomeConfig: `{"board":"esp32-c3","components":[]}`,
		PSK:           []byte{},
	}))

	got, err := database.GetDevice("dev-esph")
	require.NoError(t, err)
	assert.Equal(t, "esphome", got.FirmwareType)
	assert.Equal(t, `{"board":"esp32-c3","components":[]}`, got.ESPHomeConfig)
	assert.Equal(t, "", got.TemplateID)
}

func TestDevice_ESPHomeShownInList(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	require.NoError(t, database.CreateDevice(db.Device{
		ID: "dev-esph", Name: "Sensor", FirmwareType: "esphome", PSK: []byte{},
	}))

	devs, err := database.ListDevices()
	require.NoError(t, err)
	require.Len(t, devs, 1)
	assert.Equal(t, "esphome", devs[0].FirmwareType)
}

func TestDevice_ESPHomeAPIKey(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	require.NoError(t, database.CreateDevice(db.Device{
		ID: "dev-esph", Name: "Sensor", FirmwareType: "esphome",
		ESPHomeAPIKey: "apikey123",
		ESPHomeConfig: `{"ota_password":"otapass"}`,
		PSK:           []byte{},
	}))

	got, err := database.GetDevice("dev-esph")
	require.NoError(t, err)
	assert.Equal(t, "apikey123", got.ESPHomeAPIKey)
	assert.Equal(t, `{"ota_password":"otapass"}`, got.ESPHomeConfig)
}

func TestESPHomeJob_CRUD(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	job := db.ESPHomeJob{
		ID:         "aabbcc",
		DeviceName: "Hub4",
		ConfigJSON: `{"board":"esp32-c3"}`,
		Status:     "pending",
	}
	require.NoError(t, database.CreateJob(job))

	got, err := database.GetJob("aabbcc")
	require.NoError(t, err)
	assert.Equal(t, "aabbcc", got.ID)
	assert.Equal(t, "Hub4", got.DeviceName)
	assert.Equal(t, "pending", got.Status)

	require.NoError(t, database.UpdateJobStatus("aabbcc", "running", "", ""))
	got, err = database.GetJob("aabbcc")
	require.NoError(t, err)
	assert.Equal(t, "running", got.Status)

	require.NoError(t, database.AppendJobLog("aabbcc", "line1"))
	require.NoError(t, database.AppendJobLog("aabbcc", "line2"))
	got, err = database.GetJob("aabbcc")
	require.NoError(t, err)
	assert.Contains(t, got.Log, "line1")
	assert.Contains(t, got.Log, "line2")

	require.NoError(t, database.UpdateJobDone("aabbcc", "/data/esphome-builds/aabbcc.bin", "dev-1"))
	got, err = database.GetJob("aabbcc")
	require.NoError(t, err)
	assert.Equal(t, "done", got.Status)
	assert.Equal(t, "/data/esphome-builds/aabbcc.bin", got.BinaryPath)
	assert.Equal(t, "dev-1", got.DeviceID)

	list, err := database.ListJobs()
	require.NoError(t, err)
	require.Len(t, list, 1)
}

func TestESPHomeJob_ResetStale(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	for _, s := range []string{"pending", "running", "done", "failed"} {
		require.NoError(t, database.CreateJob(db.ESPHomeJob{
			ID: s, DeviceName: "d", ConfigJSON: "{}", Status: s,
		}))
	}
	require.NoError(t, database.ResetStaleJobs())

	for _, id := range []string{"pending", "running"} {
		got, err := database.GetJob(id)
		require.NoError(t, err)
		assert.Equal(t, "failed", got.Status, "job %s should be failed after reset", id)
	}
	for _, id := range []string{"done", "failed"} {
		got, err := database.GetJob(id)
		require.NoError(t, err)
		assert.Equal(t, id, got.Status, "job %s should be unchanged after reset", id)
	}
}

func TestESPHomeJob_DeleteOld(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	require.NoError(t, database.CreateJob(db.ESPHomeJob{
		ID: "old", DeviceName: "d", ConfigJSON: "{}", Status: "done",
	}))
	// Use a future cutoff to delete the just-created job
	require.NoError(t, database.DeleteOldJobs(time.Now().Add(time.Hour)))
	_, err = database.GetJob("old")
	assert.Error(t, err, "old job should be deleted")
}
