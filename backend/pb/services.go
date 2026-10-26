package pb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type ServiceReading struct {
	DeviceID  string    `json:"device_id"`
	State     string    `json:"state"`
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	HostIP    string    `json:"host_ip"`
	HostName  string    `json:"host_name"`
	URL       string    `json:"url,omitempty"`
	Status    string    `json:"status"`
	Detail    string    `json:"detail"`
	LatencyMS int       `json:"latency_ms"`
	CheckedAt time.Time `json:"checked_at"`
}
type ServiceSnapshot struct {
	CheckedAt time.Time        `json:"checked_at"`
	Services  []ServiceReading `json:"services"`
	Stale     bool             `json:"stale"`
}

func readServiceSnapshot(filename string, now time.Time) (ServiceSnapshot, error) {
	var value ServiceSnapshot
	file, err := os.Open(filename)
	if err != nil {
		return value, err
	}
	defer file.Close()
	if err := json.NewDecoder(io.LimitReader(file, 65536)).Decode(&value); err != nil {
		return value, err
	}
	if value.CheckedAt.IsZero() || len(value.Services) > 64 {
		return value, fmt.Errorf("invalid service snapshot")
	}
	value.Stale = now.Sub(value.CheckedAt) > 90*time.Second || value.CheckedAt.After(now.Add(10*time.Second))
	for i := range value.Services {
		if value.Stale || value.Services[i].CheckedAt.IsZero() || now.Sub(value.Services[i].CheckedAt) > 90*time.Second {
			value.Services[i].Status = "stale"
			value.Services[i].Detail = "No fresh result from Pi collector"
		}
	}
	return value, nil
}

// HandlerServices joins checks to authorized inventory IDs; legacy IP-only checks resolve only when no ID was supplied.
// e supplies auth and the collector snapshot; invalid or unauthorized IDs never fall back to an IP.
func HandlerServices(e *core.RequestEvent) error {
	e.Response.Header().Set("Cache-Control", "no-store")
	now := time.Now().UTC()
	value, err := readServiceSnapshot(os.Getenv("UPSNAP_SERVICES_FILE"), now)
	if err != nil {
		return apis.NewApiError(http.StatusServiceUnavailable, "Service checks are unavailable.", nil)
	}
	inventory, _, err := deviceInventory(e, now)
	if err != nil {
		return apis.NewApiError(503, "Device inventory unavailable.", nil)
	}
	byID := map[string]DeviceEnvelope{}
	legacyIPs := map[string]string{}
	for _, device := range inventory {
		byID[device.ID] = device
		if device.Driver == "computer" {
			legacyIPs[device.Address] = device.ID
		}
	}
	visible := make([]ServiceReading, 0, len(value.Services))
	for _, service := range value.Services {
		reference := service.DeviceID
		if reference == "" {
			reference = legacyIPs[service.HostIP]
		}
		device, ok := byID[reference]
		if !ok {
			continue
		}
		service.DeviceID = device.ID
		service.HostName = device.Name
		service.HostIP = device.Address
		if service.Status == "down" && device.State == "offline" {
			service.Status = "offline"
			service.Detail += "; host unavailable"
		}
		service.CheckedAt = service.CheckedAt.UTC()
		service.State = normalizedState(service.Status)
		visible = append(visible, service)
	}
	value.CheckedAt = value.CheckedAt.UTC()
	value.Services = visible
	return e.JSON(http.StatusOK, value)
}
