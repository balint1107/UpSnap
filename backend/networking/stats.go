package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

type DiskStats struct {
	Mount string `json:"mount"`
	Used  uint64 `json:"used"`
	Total uint64 `json:"total"`
}

type HostStats struct {
	Available   bool        `json:"available"`
	CPUPercent  float64     `json:"cpu_percent"`
	MemoryUsed  uint64      `json:"memory_used"`
	MemoryTotal uint64      `json:"memory_total"`
	Disks       []DiskStats `json:"disks"`
	CollectedAt time.Time   `json:"collected_at"`
}

type statsEntry struct {
	mu    sync.Mutex
	value HostStats
}

var statsCache sync.Map

// ReadHostStats uses only the dedicated forced SSH command; it never wakes hosts.
// Results are memory-only and shared between browser sessions for fifteen seconds.
func ReadHostStats(ctx context.Context, device *core.Record) (HostStats, error) {
	config := os.Getenv("UPSNAP_STATS_SSH_CONFIG")
	if config == "" {
		return HostStats{}, fmt.Errorf("SSH resource collection is not configured")
	}
	ip, err := ResolveToIPAddr(device.GetString("ip"))
	if err != nil || ip == "" {
		return HostStats{}, fmt.Errorf("invalid host address for resource collection")
	}
	cached, _ := statsCache.LoadOrStore(device.Id+"/"+ip, &statsEntry{})
	entry := cached.(*statsEntry)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.value.Available && time.Since(entry.value.CollectedAt) < 15*time.Second {
		return entry.value, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ssh", "-F", config,
		"-o", "ConnectTimeout=2", "-o", "ServerAliveInterval=2",
		"-o", "ServerAliveCountMax=1", ip, "stats").Output()
	if err != nil {
		return HostStats{}, fmt.Errorf("host resource command failed: %w", err)
	}
	var value HostStats
	if err := json.Unmarshal(output, &value); err != nil {
		return HostStats{}, fmt.Errorf("invalid resource response: %w", err)
	}
	if value.CPUPercent < 0 || value.CPUPercent > 100 || value.MemoryTotal == 0 || value.MemoryUsed > value.MemoryTotal {
		return HostStats{}, fmt.Errorf("resource values outside valid ranges")
	}
	for _, disk := range value.Disks {
		if disk.Total == 0 || disk.Used > disk.Total {
			return HostStats{}, fmt.Errorf("disk values outside valid ranges")
		}
	}
	value.Available = true
	value.CollectedAt = time.Now().UTC()
	entry.value = value
	return value, nil
}
