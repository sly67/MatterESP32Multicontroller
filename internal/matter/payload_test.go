package matter_test

import (
	"strings"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/matter"
	"github.com/stretchr/testify/assert"
)

func TestSetupQRPayload_Format(t *testing.T) {
	payload := matter.SetupQRPayload(3840, 20202021)
	assert.True(t, strings.HasPrefix(payload, "MT:"), "must start with MT:")
	// 3 prefix + 19 Base38 chars = 22 total (11-byte payload, 3-byte chunked)
	assert.Equal(t, 22, len(payload), "MT: payload must be 22 chars (3 prefix + 19 Base38)")
}

func TestSetupQRPayload_KnownVector(t *testing.T) {
	// discriminator=3840, passcode=20202021, VID=0xFFF1, PID=0x8000, BLE rendezvous
	// Independently computed from the 88-bit spec layout.
	assert.Equal(t, "MT:Y.K9042C00KA0648G00", matter.SetupQRPayload(3840, 20202021))
}

func TestSetupQRPayload_Deterministic(t *testing.T) {
	a := matter.SetupQRPayload(1234, 56789012)
	b := matter.SetupQRPayload(1234, 56789012)
	assert.Equal(t, a, b)
}

func TestSetupQRPayload_DifferentInputs(t *testing.T) {
	a := matter.SetupQRPayload(1234, 56789012)
	b := matter.SetupQRPayload(5678, 56789012)
	assert.NotEqual(t, a, b, "different discriminators must produce different payloads")
}

func TestSetupQRPayload_OnlyBase38Chars(t *testing.T) {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-."
	payload := matter.SetupQRPayload(2048, 12345678)
	for _, ch := range payload[3:] {
		assert.Contains(t, alphabet, string(ch), "char %q not in Base38 alphabet", ch)
	}
}
