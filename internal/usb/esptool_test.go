package usb_test

import (
	"testing"

	"github.com/karthangar/matteresp32hub/internal/usb"
	"github.com/stretchr/testify/assert"
)

func TestEsptool_PackageCompiles(t *testing.T) {
	// Full integration requires hardware. Verify package compiles correctly.
	_ = usb.GetChipInfo
	_ = usb.WriteFlash
	_ = usb.EraseFlash
	assert.True(t, true, "esptool wrapper compiles")
}
