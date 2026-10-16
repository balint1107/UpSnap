package pb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServiceCollectorFailureMarksSnapshotStale(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "status.json")
	now := time.Now().UTC()
	for _, age := range []time.Duration{30 * time.Second, 2 * time.Minute, -time.Minute} {
		value := ServiceSnapshot{CheckedAt: now.Add(-age), Services: []ServiceReading{{ID: "test", Status: "up", CheckedAt: now.Add(-age)}}}
		data, _ := json.Marshal(value)
		if err := os.WriteFile(filename, data, 0600); err != nil {
			t.Fatal(err)
		}
		result, err := readServiceSnapshot(filename, now)
		if err != nil {
			t.Fatal(err)
		}
		stale := age > 90*time.Second || age < -10*time.Second
		if result.Stale != stale || (stale && result.Services[0].Status != "stale") {
			t.Fatalf("age %v: %+v", age, result)
		}
	}
	if err := os.WriteFile(filename, []byte("broken JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readServiceSnapshot(filename, now); err == nil {
		t.Fatal("corrupt snapshot must fail closed")
	}
}
