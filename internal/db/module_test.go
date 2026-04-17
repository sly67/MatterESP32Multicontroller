package db_test

import (
	"testing"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModule_CreateAndList(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	mod := db.ModuleRow{
		ID:       "drv8833",
		Name:     "DRV8833",
		Category: "driver",
		Builtin:  true,
		YAMLBody: "id: drv8833\n",
	}
	require.NoError(t, database.CreateModule(mod))

	list, err := database.ListModules()
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "drv8833", list[0].ID)
	assert.True(t, list[0].Builtin)
}

func TestModule_GetByID(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	require.NoError(t, database.CreateModule(db.ModuleRow{
		ID: "bh1750", Name: "BH1750", Category: "sensor", YAMLBody: "id: bh1750\n",
	}))
	m, err := database.GetModule("bh1750")
	require.NoError(t, err)
	assert.Equal(t, "sensor", m.Category)
}

func TestModule_DeleteRemoves(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	require.NoError(t, database.CreateModule(db.ModuleRow{
		ID: "x", Name: "X", Category: "io", YAMLBody: "id: x\n",
	}))
	require.NoError(t, database.DeleteModule("x"))
	list, err := database.ListModules()
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestTemplate_CreateAndList(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	tpl := db.TemplateRow{
		ID:       "firefly-v1",
		Name:     "Firefly Hub v1",
		Board:    "esp32-c3",
		YAMLBody: "id: firefly-v1\n",
	}
	require.NoError(t, database.CreateTemplate(tpl))
	list, err := database.ListTemplates()
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "firefly-v1", list[0].ID)
}

func TestEffect_CreateAndList(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	eff := db.EffectRow{
		ID:       "firefly-effect",
		Name:     "Firefly Blink",
		Builtin:  true,
		YAMLBody: "id: firefly-effect\n",
	}
	require.NoError(t, database.CreateEffect(eff))
	list, err := database.ListEffects()
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "firefly-effect", list[0].ID)
}
