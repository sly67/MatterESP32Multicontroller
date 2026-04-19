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
	// 3 prefix chars + 11 Base38 chars (7 bytes: 3 pairs × 3 chars + 1 byte × 2 chars) = 14 total
	assert.Equal(t, 14, len(payload), "MT: payload must be 14 chars")
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
