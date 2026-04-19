package matter

// base38Chars is the Matter Base38 encoding alphabet (38 characters).
const base38Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-."

// base38Encode11 encodes a 64-bit integer as exactly 11 Base38 characters
// using little-endian digit order, per Matter core spec §5.1.3.1.
// Only the low 54 bits are significant; the value must fit in 38^11.
func base38Encode11(val uint64) string {
	out := make([]byte, 11)
	for i := range out {
		out[i] = base38Chars[val%38]
		val /= 38
	}
	return string(out)
}

// SetupQRPayload returns the "MT:XXXX" Matter setup QR payload string.
// discriminator is 12-bit (0–4095); passcode is 27-bit (1–99999998).
// Uses standard commissioning flow with WiFi discovery capability.
func SetupQRPayload(discriminator uint16, passcode uint32) string {
	const (
		version    = 0 // 3 bits: spec version 0
		commFlow   = 0 // 2 bits: 0 = standard commissioning flow
		rendezvous = 4 // 10 bits: bit 2 set = WiFi onboarding
	)
	// Pack fields into a 54-bit integer per Matter core spec §5.1.3.1
	raw := uint64(version) |
		uint64(commFlow)<<3 |
		uint64(rendezvous)<<5 |
		uint64(discriminator&0x0FFF)<<15 |
		uint64(passcode&0x07FFFFFF)<<27
	return "MT:" + base38Encode11(raw)
}
