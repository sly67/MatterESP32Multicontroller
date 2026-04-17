package db_test

import (
	"testing"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirmware_CreateAndList(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	fw := db.FirmwareRow{
		Version:  "1.0.0",
		Boards:   "esp32-c3,esp32-h2",
		Notes:    "Initial release",
		IsLatest: true,
	}
	require.NoError(t, database.CreateFirmware(fw))

	list, err := database.ListFirmware()
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "1.0.0", list[0].Version)
	assert.True(t, list[0].IsLatest)
}

func TestFirmware_GetLatest(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	require.NoError(t, database.CreateFirmware(db.FirmwareRow{Version: "0.9.0", Boards: "esp32-c3"}))
	require.NoError(t, database.CreateFirmware(db.FirmwareRow{Version: "1.0.0", Boards: "esp32-c3", IsLatest: true}))

	fw, err := database.GetLatestFirmware()
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", fw.Version)
}

func TestFirmware_SetLatest(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	require.NoError(t, database.CreateFirmware(db.FirmwareRow{Version: "1.0.0", Boards: "esp32-c3", IsLatest: true}))
	require.NoError(t, database.CreateFirmware(db.FirmwareRow{Version: "1.1.0", Boards: "esp32-c3"}))
	require.NoError(t, database.SetLatestFirmware("1.1.0"))

	fw, err := database.GetLatestFirmware()
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", fw.Version)

	old, err := database.GetFirmware("1.0.0")
	require.NoError(t, err)
	assert.False(t, old.IsLatest)
}
