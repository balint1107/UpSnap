package networking

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestRebootOnlyPowerPolicy(t *testing.T) {
	device := core.NewRecord(&core.Collection{})
	device.Set("reboot_only", true)
	device.Set("ip", "192.168.50.152")
	if WakeDevice(nil, device, "test") == nil {
		t.Fatal("wake must be rejected before network activity")
	}
	if ShutdownDevice(nil, device, "test") == nil {
		t.Fatal("suspend must be rejected before network activity")
	}
	if _, err := SleepDevice(nil, device); err == nil {
		t.Fatal("sleep must be rejected")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "args")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("UPSNAP_STATS_SSH_CONFIG", "/test/config")
	t.Setenv("REBOOT_TEST_LOG", log)
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$REBOOT_TEST_LOG\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := QueueHostReboot(context.Background(), device); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil || !strings.HasSuffix(string(data), "192.168.50.152\nreboot\n") || !strings.Contains(string(data), "/test/config\n") {
		t.Fatalf("reboot must use the fixed restricted SSH action: %q %v", data, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if QueueHostReboot(ctx, device) == nil {
		t.Fatal("cancelled reboot request must fail")
	}
}
