package pb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// PowerSample is a public instantaneous watts sample at Unix time T (seconds).
type PowerSample struct {
	T int64   `json:"t"`
	W float64 `json:"w"`
}

// EnergyDay holds the observed device-calendar total in kWh for a Budapest date.
type EnergyDay struct {
	Date string  `json:"date"`
	KWh  float64 `json:"kwh"`
}

// PlugReading contains nullable physical units; nil means unavailable, never zero.
type PlugReading struct {
	PowerW   *float64 `json:"power_w"`
	Today    *float64 `json:"energy_today_kwh"`
	Month    *float64 `json:"energy_month_kwh"`
	Voltage  *float64 `json:"voltage_v"`
	Current  *float64 `json:"current_a"`
	Relay    *bool    `json:"relay_on"`
	Model    string   `json:"model"`
	Hardware string   `json:"hardware"`
	Firmware string   `json:"firmware"`
}

// PowerDevice separates a read-only outlet from SSH-managed hosts and their controls.
type PowerDevice struct {
	Device            *DeviceEnvelope `json:"device,omitempty"`
	CollectorDeviceID string          `json:"collector_device_id,omitempty"`
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	IP                string          `json:"ip"`
	HostIP            string          `json:"host_ip"`
	Status            string          `json:"status"`
	Detail            string          `json:"detail"`
	CheckedAt         time.Time       `json:"checked_at"`
	Reading           PlugReading     `json:"reading"`
	History           []PowerSample   `json:"history"`
	Daily             []EnergyDay     `json:"daily"`
}

// PowerSnapshot is the Pi collector's bounded power-device export.
type PowerSnapshot struct {
	CheckedAt time.Time     `json:"checked_at"`
	Devices   []PowerDevice `json:"devices"`
}

// readPowerSnapshot reads filename (maximum 1 MiB) and marks readings stale against UTC now.
// It returns an error for malformed, oversized or unbounded snapshots; history remains visible when stale.
func readPowerSnapshot(filename string, now time.Time) (PowerSnapshot, error) {
	var value PowerSnapshot
	file, err := os.Open(filename)
	if err != nil {
		return value, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 1048577))
	if err != nil {
		return value, err
	}
	if len(data) > 1048576 {
		return value, fmt.Errorf("power snapshot too large")
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, err
	}
	if value.CheckedAt.IsZero() || len(value.Devices) > 8 {
		return value, fmt.Errorf("invalid power snapshot")
	}
	stale := now.Sub(value.CheckedAt) > 90*time.Second || value.CheckedAt.After(now.Add(10*time.Second))
	for i := range value.Devices {
		d := &value.Devices[i]
		if len(d.History) > 1441 || len(d.Daily) > 30 {
			return value, fmt.Errorf("power history exceeds bounds")
		}
		if stale || d.CheckedAt.IsZero() || now.Sub(d.CheckedAt) > 90*time.Second || d.CheckedAt.After(now.Add(10*time.Second)) {
			d.Status = "stale"
			d.Detail = "No fresh result from Pi collector"
		}
	}
	return value, nil
}

// HandlerPower returns power devices/history only to users who can read their collecting host.
// Auth middleware is required; no credentials or power-control endpoint are exposed.
func HandlerPower(e *core.RequestEvent) error {
	e.Response.Header().Set("Cache-Control", "no-store")
	filename := os.Getenv("UPSNAP_SERVICES_FILE")
	if filename == "" {
		return apis.NewApiError(http.StatusServiceUnavailable, "Power readings unavailable.", nil)
	}
	value, err := readPowerSnapshot(filepath.Join(filepath.Dir(filename), "power.json"), time.Now())
	if err != nil {
		return apis.NewApiError(http.StatusServiceUnavailable, "Power readings unavailable.", nil)
	}
	visible := make([]PowerDevice, 0, len(value.Devices))
	for _, plug := range value.Devices {
		host, err := inventoryHost(e.App, plug.CollectorDeviceID, plug.HostIP)
		if err != nil {
			continue
		}
		access := deviceAccess(e, host)
		if !access.Read {
			continue
		}
		plug.CollectorDeviceID = deviceReference("computer", host.Id)
		common := powerEnvelope(plug, plug.CollectorDeviceID, access, time.Now())
		plug.Device = &common
		visible = append(visible, plug)
	}
	value.Devices = visible
	return e.JSON(http.StatusOK, value)
}
