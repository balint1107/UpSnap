package networking

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// TestSuspendNetworkProbeSkipsBootProfiles proves malformed SSH config cannot substitute for raw network reachability.
func TestSuspendNetworkProbeSkipsBootProfiles(t *testing.T) {
	t.Setenv("UPSNAP_BOOT_HOSTS_FILE", filepath.Join(t.TempDir(), "absent.json"))
	device := core.NewRecord(&core.Collection{})
	device.Set("ip", "")
	up, err := PingDeviceNetwork(device)
	var pathErr *os.PathError
	if up || errors.As(err, &pathErr) {
		t.Fatalf("raw probe consulted boot profiles: up=%v err=%v", up, err)
	}
}

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
