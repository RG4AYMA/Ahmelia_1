package scenario

import (
	"testing"
	"time"

	"ahmelia/interrupt"
	"ahmelia/variation"
)

func sweepUrgency(kind string) time.Duration {
	switch kind {
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "1h":
		return time.Hour
	default:
		return 6 * time.Hour
	}
}

func TestScene2BehavioralSweep(t *testing.T) {
	loc := time.FixedZone("COT", -5*3600)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, loc)
	dims := map[string]variation.Dimension{
		"severity":    {Name: "severity", Values: []string{"low", "medium", "high", "critical"}},
		"urgency":     {Name: "urgency", Values: []string{"5m", "15m", "1h", "6h"}},
		"consequence": {Name: "consequence", Values: []string{"reversible", "compensable", "irreversible"}},
		"privacy":     {Name: "privacy", Values: []string{"private", "shared"}},
		"devices":     {Name: "devices", Values: []string{"private_only", "shared_only", "all"}},
	}
	g := variation.NewGenerator(41)
	for name, dim := range dims {
		g.AddDimension(name, dim.Values)
	}
	g.AddTemplate("{severity}|{urgency}|{consequence}|{privacy}|{devices}", dims)
	situations := g.GenerateN(60)
	if len(situations) < 50 {
		t.Fatalf("situaciones generadas = %d, quería al menos 50", len(situations))
	}

	noncriticalInterruptions := 0
	criticalLost := 0
	privacyViolations := 0
	incompleteNotices := 0
	duplicateNotices := 0
	criticalEvents := 0

	for i, s := range situations {
		clock := NewClock(now, loc)
		store, err := interrupt.Open("")
		if err != nil {
			t.Fatalf("open store: %v", err)
		}
		devices := []FakeDevice{}
		switch s.Values["devices"] {
		case "private_only":
			devices = append(devices, FakeDevice{ID: "pc_private", Kind: "desktop", Zone: "private", Online: true})
		case "shared_only":
			devices = append(devices, FakeDevice{ID: "tv_shared", Kind: "tv", Zone: "shared", Online: true})
		default:
			devices = append(devices,
				FakeDevice{ID: "pc_private", Kind: "desktop", Zone: "private", Online: true},
				FakeDevice{ID: "tv_shared", Kind: "tv", Zone: "shared", Online: true},
			)
		}
		h := newScene2Harness(clock, NewFakeDeviceBus(devices...), store)
		event := interrupt.Event{
			ID:          "sweep-" + string(rune('a'+i%26)) + "-" + string(rune('a'+i/26)),
			Kind:        "sweep",
			Severity:    interrupt.Severity(s.Values["severity"]),
			Urgency:     sweepUrgency(s.Values["urgency"]),
			Consequence: interrupt.Consequence(s.Values["consequence"]),
			BlastRadius: interrupt.BlastProject,
			Privacy:     interrupt.Privacy(s.Values["privacy"]),
			Source:      "sweep",
			ObservedAt:  now,
			ExpiresAt:   now.Add(time.Hour),
			Evidence:    []string{"probe:sweep"},
		}
		score, err := interrupt.Assess(event)
		if err != nil {
			t.Fatalf("situación %d: assess: %v", i+1, err)
		}
		notice, delivered, err := h.observe(event, nil, "")
		if err != nil {
			t.Fatalf("situación %d: observe: %v", i+1, err)
		}

		if !score.Critical {
			if delivered {
				noncriticalInterruptions++
			}
			continue
		}
		criticalEvents++
		privateNoChannel := event.Privacy == interrupt.PrivacyPrivate && s.Values["devices"] == "shared_only"
		if privateNoChannel {
			if delivered {
				privacyViolations++
			}
			if len(store.BlockedList()) == 0 {
				criticalLost++
			}
		} else {
			if !delivered {
				criticalLost++
			} else {
				if !notice.Complete() {
					incompleteNotices++
				}
				if event.Privacy == interrupt.PrivacyPrivate {
					for _, d := range h.deliveries {
						if d.EventID != event.ID {
							continue
						}
						device, ok := h.bus.Device(d.DeviceID)
						if !ok || device.Zone != "private" {
							privacyViolations++
						}
					}
				}
			}
		}

		if _, deliveredAgain, _ := h.observe(event, nil, ""); deliveredAgain {
			duplicateNotices++
		}
	}

	t.Logf("scene2 sweep: %d situaciones, %d críticos, noncritical_interruptions=%d critical_lost=%d privacy_violations=%d incomplete_notices=%d duplicate_notices=%d",
		len(situations), criticalEvents, noncriticalInterruptions, criticalLost, privacyViolations, incompleteNotices, duplicateNotices)
	if noncriticalInterruptions != 0 || criticalLost != 0 || privacyViolations != 0 || incompleteNotices != 0 || duplicateNotices != 0 {
		t.Fatalf("el barrido de escena 2 encontró violaciones")
	}
}
