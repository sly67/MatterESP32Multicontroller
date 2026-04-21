package flash

import (
	"fmt"
	"io"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/esphome"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
)

// ESPHomeRequest holds the parameters for an ESPHome flash operation.
type ESPHomeRequest struct {
	Port         string
	DeviceName   string
	WiFiSSID     string
	WiFiPassword string
	Board        string
	HAIntegration bool
	Components   []esphome.ComponentConfig
}

// FlashESPHomeDevice is a stub preserved for interface compatibility.
// TODO(Task 6): replace with queue-based implementation.
func FlashESPHomeDevice(_ *db.Database, _ map[string]*yamldef.Module, _ ESPHomeRequest, _ io.Writer) Result {
	return Result{Error: fmt.Errorf("ESPHome USB flash not yet implemented — use /api/jobs")}
}
