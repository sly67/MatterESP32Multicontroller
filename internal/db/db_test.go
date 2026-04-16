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
