package nvs

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/karthangar/matteresp32hub/internal/yamldef"
)

// Compile produces an ESP-IDF NVS partition CSV string from a template and device config.
// The CSV is suitable as input to nvs_partition_gen.py.
//
// NVS key names are limited to 15 characters. GPIO values are stored as strings
// (e.g. "GPIO4"). The PSK is base64-encoded as a binary blob.
func Compile(tpl *yamldef.Template, dev DeviceConfig) (string, error) {
	if len(dev.PSK) == 0 {
		return "", fmt.Errorf("PSK must not be empty")
	}

	var b strings.Builder
	row := func(key, typ, enc, val string) {
		b.WriteString(key)
		b.WriteByte(',')
		b.WriteString(typ)
		b.WriteByte(',')
		b.WriteString(enc)
		b.WriteByte(',')
		b.WriteString(val)
		b.WriteByte('\n')
	}
	ns := func(name string) { row(name, "namespace", "", "") }

	b.WriteString("key,type,encoding,value\n")

	// wifi namespace
	ns("wifi")
	row("ssid", "data", "string", dev.WiFiSSID)
	row("pass", "data", "string", dev.WiFiPassword)

	// security namespace
	ns("security")
	row("psk", "data", "base64", base64.StdEncoding.EncodeToString(dev.PSK))

	// hw namespace
	ns("hw")
	row("board", "data", "string", dev.BoardID)

	// device namespace
	ns("device")
	row("name", "data", "string", dev.Name)

	// matter namespace
	ns("matter")
	row("disc", "data", "u16", fmt.Sprintf("%d", dev.MatterDiscrim))
	row("passcode", "data", "u32", fmt.Sprintf("%d", dev.MatterPasscode))

	// modules namespace: count
	ns("modules_cfg")
	row("count", "data", "u8", fmt.Sprintf("%d", len(tpl.Modules)))

	// per-module namespaces
	for i, tm := range tpl.Modules {
		nsName := fmt.Sprintf("mod_%d", i)
		if len(nsName) > 15 {
			return "", fmt.Errorf("module namespace %q exceeds 15-char NVS limit", nsName)
		}
		ns(nsName)
		row("type", "data", "string", tm.Module)
		row("ep_name", "data", "string", tm.EndpointName)
		if tm.Effect != "" {
			row("effect", "data", "string", tm.Effect)
		}
		// Pin assignments: key is "p_" + pinID truncated to 15 chars
		for pinID, gpio := range tm.Pins {
			key := "p_" + pinID
			if len(key) > 15 {
				key = key[:15]
			}
			row(key, "data", "string", gpio)
		}
	}

	return b.String(), nil
}
