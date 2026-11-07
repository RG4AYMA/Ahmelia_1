package scenario

import (
	"fmt"
	"sync"
	"time"
)

// FakeDevice is a deterministic stand-in for a real endpoint. It never sleeps:
// latency is recorded, not waited.
type FakeDevice struct {
	ID       string
	Kind     string
	Zone     string
	Online   bool
	Latency  time.Duration
	FailNext bool
}

// DeviceAction is the observable result of one delivery attempt.
type DeviceAction struct {
	DeviceID string
	Action   string
	Executed bool
	At       time.Time
	Detail   string
}

// FakeDeviceBus is a test double, not a production device registry.
type FakeDeviceBus struct {
	mu      sync.Mutex
	devices map[string]*FakeDevice
	actions []DeviceAction
}

// NewFakeDeviceBus builds a bus with the given devices.
func NewFakeDeviceBus(devices ...FakeDevice) *FakeDeviceBus {
	b := &FakeDeviceBus{devices: make(map[string]*FakeDevice, len(devices))}
	for i := range devices {
		d := devices[i]
		b.devices[d.ID] = &d
	}
	return b
}

// Execute records one attempt. A missing or offline device fails; a device
// with FailNext fails once and then recovers.
func (b *FakeDeviceBus) Execute(deviceID, action string, now time.Time) (DeviceAction, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := DeviceAction{DeviceID: deviceID, Action: action, At: now}
	device, ok := b.devices[deviceID]
	if !ok {
		result.Detail = "unknown device"
		b.actions = append(b.actions, result)
		return result, fmt.Errorf("device %q not found", deviceID)
	}
	if !device.Online {
		result.Detail = "offline"
		b.actions = append(b.actions, result)
		return result, fmt.Errorf("device %q offline", deviceID)
	}
	if device.FailNext {
		device.FailNext = false
		result.Detail = "transient failure"
		b.actions = append(b.actions, result)
		return result, fmt.Errorf("device %q transient failure", deviceID)
	}
	result.Executed = true
	result.Detail = "delivered"
	if device.Latency > 0 {
		result.Detail = "delivered; simulated_latency=" + device.Latency.String()
	}
	b.actions = append(b.actions, result)
	return result, nil
}

// Actions returns every attempt, including failures.
func (b *FakeDeviceBus) Actions() []DeviceAction {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]DeviceAction(nil), b.actions...)
}

// Device returns a copy of one device.
func (b *FakeDeviceBus) Device(id string) (FakeDevice, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d, ok := b.devices[id]
	if !ok {
		return FakeDevice{}, false
	}
	return *d, true
}
