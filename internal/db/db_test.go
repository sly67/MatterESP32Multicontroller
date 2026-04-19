package db_test

import (
	"testing"

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
