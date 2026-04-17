package usb_test

import (
	"testing"

	"github.com/karthangar/matteresp32hub/internal/usb"
	"github.com/stretchr/testify/require"
)

func TestListPorts_NoError(t *testing.T) {
	ports, err := usb.ListPorts()
	require.NoError(t, err)
	_ = ports // may be empty on test host
}
