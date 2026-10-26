//go:build linux

package networking

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/pocketbase/pocketbase/core"
	probing "github.com/prometheus-community/pro-bing"
	"kernel.org/pub/linux/libs/security/libcap/cap"
)

// PingDevice probes device readiness; device is borrowed and never mutated.
// Returns authenticated readiness (dual-boot profiles when configured), or a probe error.
func PingDevice(device *core.Record) (bool, error) {
	return pingDevice(device, true)
}

// pingDevice probes borrowed device; identity selects authenticated boot readiness or raw network reachability.
// Returns reachability and a probe error; false identity skips custom commands through the caller's clean record.
func pingDevice(device *core.Record, identity bool) (bool, error) {
	if identity {
		host, cfgErr := BootConfiguration(device.GetString("ip"))
		if cfgErr != nil {
			return false, cfgErr
		}
		if host != nil {
			_, err := DetectBootOS(context.Background(), device.GetString("ip"))
			return err == nil, nil
		}
	}
	ping_cmd := device.GetString("ping_cmd")
	if ping_cmd == "" {
		ip, err := ResolveToIPAddr(device.GetString("ip"))
		if err != nil {
			return false, err
		}
		if ip == "" {
			return false, nil
		}
		pinger, err := probing.NewPinger(ip)
		if err != nil {
			return false, err
		}
		pinger.Count = 1
		pinger.Timeout = 500 * time.Millisecond

		privileged := true
		privilegedEnv := os.Getenv("UPSNAP_PING_PRIVILEGED")
		if privilegedEnv != "" {
			privileged, err = strconv.ParseBool(privilegedEnv)
			if err != nil {
				privileged = false
			}
		}
		if privileged {
			orig := cap.GetProc()
			defer orig.SetProc() // restore original caps on exit.

			c, err := orig.Dup()
			if err != nil {
				return false, fmt.Errorf("Failed to dup existing capabilities: %v", err)
			}

			if on, _ := c.GetFlag(cap.Permitted, cap.NET_RAW); !on {
				return false, fmt.Errorf("Privileged ping selected but NET_RAW capability not permitted")
			}

			if err := c.SetFlag(cap.Effective, true, cap.NET_RAW); err != nil {
				return false, fmt.Errorf("unable to set NET_RAW capability")
			}

			if err := c.SetProc(); err != nil {
				return false, fmt.Errorf("unable to raise NET_RAW capability")
			}
		}
		pinger.SetPrivileged(privileged)

		err = pinger.Run()
		if err != nil {
			if isNoRouteOrDownError(err) {
				return false, nil
			}
			return false, err
		}
		stats := pinger.Statistics()
		return stats.PacketLoss == 0, nil
	} else {
		var shell string
		var shell_arg string

		shell = "/bin/sh"
		shell_arg = "-c"

		cmd := exec.Command(shell, shell_arg, ping_cmd)
		err := cmd.Run()

		if err != nil {
			if _, ok := err.(*exec.ExitError); ok {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}
}
