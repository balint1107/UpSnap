package networking

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// QueueHostReboot uses borrowed device configuration and ctx cancellation to queue a fixed Pi reboot.
// Returns an error for managed dual-boot hosts, unsupported records, or failed SSH; never falls back to suspend.
func QueueHostReboot(ctx context.Context, device *core.Record) error {
	host, configErr := BootConfiguration(device.GetString("ip"))
	if configErr != nil || host != nil {
		return fmt.Errorf("dual-boot restart is locked pending explicit authorization")
	}
	if !device.GetBool("reboot_only") {
		return fmt.Errorf("host is not reboot-only")
	}
	config := os.Getenv("UPSNAP_STATS_SSH_CONFIG")
	if config == "" {
		return fmt.Errorf("restricted SSH config is not configured")
	}
	ip, err := ResolveToIPAddr(device.GetString("ip"))
	if err != nil || ip == "" {
		return fmt.Errorf("invalid reboot host address")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "ssh", "-F", config, "-o", "ConnectTimeout=2",
		"-o", "ServerAliveInterval=2", "-o", "ServerAliveCountMax=1", ip, "reboot").Run(); err != nil {
		return fmt.Errorf("could not queue host reboot: %w", err)
	}
	return nil
}
