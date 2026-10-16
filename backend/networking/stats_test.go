package networking

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestHostStatsCacheAndValidation(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("UPSNAP_STATS_SSH_CONFIG", filepath.Join(dir, "config"))
	t.Setenv("STATS_TEST_LOG", log)
	writeSSH := func(cpu string) {
		t.Helper()
		script := "#!/bin/sh\nprintf 'called\\n' >> \"$STATS_TEST_LOG\"\nprintf '%s\\n' '{\"cpu_percent\":" + cpu + ",\"memory_used\":100,\"memory_total\":1000,\"disks\":[{\"mount\":\"/\",\"used\":100,\"total\":1000}]}'\n"
		if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	device := core.NewRecord(&core.Collection{})
	device.Id = "stats-cache-test"
	device.Set("ip", "127.0.0.1")
	statsCache.Delete(device.Id + "/127.0.0.1")
	t.Cleanup(func() { statsCache.Delete(device.Id + "/127.0.0.1") })
	writeSSH("12.5")
	for range 2 {
		stats, err := ReadHostStats(context.Background(), device)
		if err != nil || !stats.Available || stats.CPUPercent != 12.5 {
			t.Fatalf("unexpected metrics: %+v, %v", stats, err)
		}
	}
	data, err := os.ReadFile(log)
	if err != nil || strings.Count(string(data), "called") != 1 {
		t.Fatalf("consecutive browser requests should share one SSH collection: %s, %v", data, err)
	}
	statsCache.Delete(device.Id + "/127.0.0.1")
	writeSSH("150")
	if _, err := ReadHostStats(context.Background(), device); err == nil {
		t.Fatal("invalid CPU readings must be rejected")
	}
}
