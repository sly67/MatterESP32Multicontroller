package matter

// base38Chars is the Matter Base38 encoding alphabet (38 characters).
const base38Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-."

// base38Encode encodes bytes using Matter's Base38 scheme.
// 3 bytes → 5 chars, 2 bytes → 4 chars, 1 byte → 2 chars.
func base38Encode(data []byte) string {
	var out []byte
	for len(data) >= 3 {
		val := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16
		for i := 0; i < 5; i++ {
			out = append(out, base38Chars[val%38])
			val /= 38
		}
		data = data[3:]
	}
	switch len(data) {
	case 2:
		val := uint32(data[0]) | uint32(data[1])<<8
		for i := 0; i < 4; i++ {
			out = append(out, base38Chars[val%38])
			val /= 38
		}
	case 1:
		val := uint32(data[0])
		for i := 0; i < 2; i++ {
			out = append(out, base38Chars[val%38])
			val /= 38
		}
	}
	return string(out)
}

// SetupQRPayload returns the "MT:XXXX" Matter setup QR payload string.
// discriminator is 12-bit (0–4095); passcode is 27-bit (1–99999998).
// Uses CSA test VID/PID (0xFFF1/0x8000) with BLE commissioning flow.
func SetupQRPayload(discriminator uint16, passcode uint32) string {
	const (
		version   = 0
		vendorID  = 0xFFF1 // CSA test VID — default for esp-matter
		productID = 0x8000 // CSA test PID
		commFlow  = 0      // standard commissioning flow
		rendezvous = 0x02  // bit 1 = BLE
	)

	// Pack 88-bit little-endian payload per Matter Core Spec §5.1.3.1:
	//   version(3) | vendorID(16) | productID(16) | commFlow(2) |
	//   rendezvous(8) | discriminator(12) | passcode(27) | padding(4)
	var buf [11]byte
	set := func(val uint64, offset, bits int) {
		for i := 0; i < bits; i++ {
			if val>>i&1 != 0 {
				buf[offset/8] |= 1 << (offset % 8)
			}
			offset++
		}
	}
	set(version, 0, 3)
	set(vendorID, 3, 16)
	set(productID, 19, 16)
	set(commFlow, 35, 2)
	set(rendezvous, 37, 8)
	set(uint64(discriminator&0x0FFF), 45, 12)
	set(uint64(passcode&0x07FFFFFF), 57, 27)

	return "MT:" + base38Encode(buf[:])
}
