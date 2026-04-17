package flash_test

import (
	"testing"

	"github.com/karthangar/matteresp32hub/internal/flash"
	"github.com/stretchr/testify/assert"
)

func TestFlash_PackageCompiles(t *testing.T) {
	var req flash.Request
	assert.Equal(t, "", req.Port)
	var res flash.Result
	assert.Nil(t, res.Error)
}
