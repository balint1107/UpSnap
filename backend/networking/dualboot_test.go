package networking

import (
	"context"
	"encoding/json"
	"github.com/pocketbase/pocketbase/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bootFixture configures two fake SSH profiles for t and returns the command log path and mutable host config path.
// All commands are local scripts; no real SSH, firmware write or power action can occur.
func bootFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	config := filepath.Join(dir, "hosts.json")
	host := BootHost{Profiles: map[string]BootProfile{"windows": {Alias: "test-windows", Username: "win"}, "ubuntu": {Alias: "test-ubuntu", Username: "linux"}}}
	data, _ := json.Marshal(map[string]BootHost{"192.168.50.148": host})
	if err := os.WriteFile(config, data, 0600); err != nil {
		t.Fatal(err)
	}
	status := `{"os":"ubuntu","ready":true,"writes_enabled":false,"restart_enabled":false,"targets":{"windows":{"available":true,"entry":"0003"},"ubuntu":{"available":true,"entry":"0000"}},"boot_next":null,"boot_order":"AAADAA==","boot_order_unchanged":true}`
	if err := os.WriteFile(filepath.Join(dir, "status.json"), []byte(status), 0600); err != nil {
		t.Fatal(err)
	}
	validated := strings.Replace(status, `"boot_next":null`, `"validated_target":"windows","boot_next":null`, 1)
	if err := os.WriteFile(filepath.Join(dir, "validated.json"), []byte(validated), 0600); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
for a do last="$a"; done
printf '%s\n' "$last" >> "$BOOT_TEST_LOG"
case "$*" in *test-windows*) exit 1;; esac
case "$last" in
 check) printf '{"os":"ubuntu","ready":true}\n';;
 boot-status) cat "$BOOT_TEST_DIR/status.json";;
 validate-windows) cat "$BOOT_TEST_DIR/validated.json";;
 restart-windows) cat "$BOOT_TEST_DIR/queued.json";;
 *) exit 64;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BOOT_TEST_LOG", log)
	t.Setenv("BOOT_TEST_DIR", dir)
	t.Setenv("UPSNAP_BOOT_HOSTS_FILE", config)
	t.Setenv("UPSNAP_STATS_SSH_CONFIG", "/fake/config")
	bootDetect.Lock()
	bootDetect.ip = ""
	bootDetect.Unlock()
	return log, config
}

// TestBootRestartChecksSourceAndQueueAcknowledgement uses only fake SSH to verify gates, source identity and native queue proof.
func TestBootRestartChecksSourceAndQueueAcknowledgement(t *testing.T) {
	log, cfg := bootFixture(t)
	if _, err := RestartBootTarget(context.Background(), "192.168.50.148", "windows", "source", "AAADAA=="); err == nil {
		t.Fatal("locked restart accepted")
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("locked restart invoked SSH")
	}
	host := BootHost{WritesEnabled: true, RestartEnabled: true, Profiles: map[string]BootProfile{"ubuntu": {Alias: "test-ubuntu", Username: "linux", HelperInstalled: true}, "windows": {Alias: "test-windows", Username: "win", HelperInstalled: true}}}
	data, _ := json.Marshal(map[string]BootHost{"192.168.50.148": host})
	if err := os.WriteFile(cfg, data, 0600); err != nil {
		t.Fatal(err)
	}
	status := BootStatus{OS: "ubuntu", Ready: true, WritesEnabled: true, RestartEnabled: true, BootID: "source", BootOrder: "AAADAA==", BootOrderUnchanged: true, Targets: map[string]BootTarget{"ubuntu": {Available: true, Entry: "0000"}, "windows": {Available: true, Entry: "0003"}}}
	next := "0003"
	status.BootNext = &next
	dir := os.Getenv("BOOT_TEST_DIR")
	data, _ = json.Marshal(status)
	if err := os.WriteFile(filepath.Join(dir, "status.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RestartBootTarget(context.Background(), "192.168.50.148", "windows", "other-boot", "AAADAA=="); err == nil {
		t.Fatal("changed source accepted")
	}
	calls, _ := os.ReadFile(log)
	if strings.Contains(string(calls), "restart-windows") {
		t.Fatal("restart ran with wrong source")
	}
	status.RestartQueued = true
	data, _ = json.Marshal(status)
	if err := os.WriteFile(filepath.Join(dir, "queued.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RestartBootTarget(context.Background(), "192.168.50.148", "windows", "source", "AAADAA=="); err != nil {
		t.Fatal(err)
	}
	status.RestartQueued = false
	data, _ = json.Marshal(status)
	if err := os.WriteFile(filepath.Join(dir, "queued.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RestartBootTarget(context.Background(), "192.168.50.148", "windows", "source", "AAADAA=="); err == nil {
		t.Fatal("missing queue proof accepted")
	}
}

// TestBootValidationAndLocks proves OS identity, target preflight and both firmware-write gates without power commands.
func TestBootValidationAndLocks(t *testing.T) {
	log, config := bootFixture(t)
	status, err := ReadBootStatus(context.Background(), "192.168.50.148")
	if err != nil || status.OS != "ubuntu" || status.RestartEnabled || status.WritesEnabled {
		t.Fatalf("unexpected status: %+v %v", status, err)
	}
	result, err := CheckBootTarget(context.Background(), "192.168.50.148", "windows", false)
	if err != nil || result.ValidatedTarget != "windows" {
		t.Fatalf("validation failed: %+v %v", result, err)
	}
	before, _ := os.ReadFile(log)
	if _, err := CheckBootTarget(context.Background(), "192.168.50.148", "windows", true); err == nil {
		t.Fatal("locked preparation accepted")
	}
	after, _ := os.ReadFile(log)
	if string(before) != string(after) {
		t.Fatal("locked preparation ran SSH")
	}
	bytes, _ := os.ReadFile(config)
	if err := os.WriteFile(config, []byte(strings.Replace(string(bytes), `"writes_enabled":false`, `"writes_enabled":true`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckBootTarget(context.Background(), "192.168.50.148", "windows", true); err == nil {
		t.Fatal("host-side write lock ignored")
	}
	final, _ := os.ReadFile(log)
	if strings.Contains(string(final), "prepare-") || strings.Contains(string(final), "reboot") {
		t.Fatal("mutating action escaped locks")
	}
	if _, err := CheckBootTarget(context.Background(), "192.168.50.148", "windows; reboot", false); err == nil {
		t.Fatal("arbitrary action accepted")
	}
}

// TestBootConfigurationRejectsUnsafeAliases ensures malformed trusted profiles cannot reach command execution.
func TestBootConfigurationRejectsUnsafeAliases(t *testing.T) {
	log, config := bootFixture(t)
	data, _ := os.ReadFile(config)
	if err := os.WriteFile(config, []byte(strings.Replace(string(data), "test-ubuntu", "-oProxyCommand=bad", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadBootStatus(context.Background(), "192.168.50.148"); err == nil {
		t.Fatal("unsafe alias accepted")
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("invalid configuration executed SSH")
	}
}

// TestBootPrepareReadbackMismatch rejects a helper that claims validation but reports a changed normal boot order.
func TestBootPrepareReadbackMismatch(t *testing.T) {
	_, _ = bootFixture(t)
	dir := os.Getenv("BOOT_TEST_DIR")
	p := filepath.Join(dir, "validated.json")
	data, _ := os.ReadFile(p)
	if err := os.WriteFile(p, []byte(strings.Replace(string(data), `"boot_order_unchanged":true`, `"boot_order_unchanged":false`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckBootTarget(context.Background(), "192.168.50.148", "windows", false); err == nil {
		t.Fatal("changed BootOrder accepted")
	}
}

// TestManagedHostCannotUseLegacyReboot ensures even a reboot-only record cannot bypass the managed-host lock.
func TestManagedHostCannotUseLegacyReboot(t *testing.T) {
	log, _ := bootFixture(t)
	device := core.NewRecord(&core.Collection{})
	device.Set("ip", "192.168.50.148")
	device.Set("reboot_only", true)
	if err := QueueHostReboot(context.Background(), device); err == nil {
		t.Fatal("managed reboot accepted")
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("managed reboot executed SSH")
	}
}

// TestWindowsCannotUseLinuxSuspendOrSOL verifies Windows identity disables all legacy suspend paths before execution.
func TestWindowsCannotUseLinuxSuspendOrSOL(t *testing.T) {
	log, _ := bootFixture(t)
	script := `#!/bin/sh
for a do last="$a"; done
printf '%s\n' "$last" >> "$BOOT_TEST_LOG"
case "$*" in *test-windows*) printf '{"os":"windows","ready":true}\n';; *) exit 1;; esac
`
	if err := os.WriteFile(filepath.Join(os.Getenv("BOOT_TEST_DIR"), "ssh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	device := core.NewRecord(&core.Collection{})
	device.Set("ip", "192.168.50.148")
	if err := ShutdownDevice(nil, device, "test"); err == nil {
		t.Fatal("Linux suspend accepted on Windows")
	}
	if _, err := SleepDevice(nil, device); err == nil {
		t.Fatal("Linux SOL accepted on Windows")
	}
	data, _ := os.ReadFile(log)
	if strings.Contains(string(data), "suspend") || strings.Contains(string(data), "reboot") {
		t.Fatal("power action executed")
	}
}
