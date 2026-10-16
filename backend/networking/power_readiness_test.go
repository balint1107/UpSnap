package networking

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestSuspendProbeIgnoresReadinessCommand(t *testing.T) {
	device := core.NewRecord(&core.Collection{})
	device.Set("ip", "")
	device.Set("ping_cmd", "exit 0")
	up, err := PingDeviceNetwork(device)
	if up {
		t.Fatalf("a missing host must not be reachable because the readiness command succeeds: up=%v err=%v", up, err)
	}
	if device.GetString("ping_cmd") != "exit 0" {
		t.Fatal("network probing changed the device readiness configuration")
	}
}
