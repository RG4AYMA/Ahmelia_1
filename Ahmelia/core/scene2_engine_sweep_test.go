package core

import (
	"testing"
	"time"

	"ahmelia/interrupt"
	"ahmelia/variation"
)

func engineSweepUrgency(kind string) time.Duration {
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

func TestScene2EngineBehavioralSweep(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	dims := map[string]variation.Dimension{
		"severity":    {Name: "severity", Values: []string{"low", "medium", "high", "critical"}},
		"urgency":     {Name: "urgency", Values: []string{"5m", "15m", "1h", "6h"}},
		"consequence": {Name: "consequence", Values: []string{"reversible", "compensable", "irreversible"}},
		"privacy":     {Name: "privacy", Values: []string{"private", "shared"}},
		"devices":     {Name: "devices", Values: []string{"private_only", "shared_only", "all"}},
		"silence":     {Name: "silence", Values: []string{"false", "true"}},
	}
	g := variation.NewGenerator(61)
	for name, dim := range dims {
		g.AddDimension(name, dim.Values)
	}
	g.AddTemplate("{severity}|{urgency}|{consequence}|{privacy}|{devices}|{silence}", dims)
	situations := g.GenerateN(60)
	if len(situations) != 60 {
		t.Fatalf("situaciones generadas = %d, quería 60", len(situations))
	}

	noncriticalInterruptions := 0
	criticalEvents := 0
	criticalLost := 0
	privacyViolations := 0
	incompleteNotices := 0
	duplicateNotices := 0
	silenceViolations := 0

	for i, s := range situations {
		bus := &fakeNoticeChannel{}
		switch s.Values["devices"] {
		case "private_only":
			bus.devices = []NoticeDevice{privateDevice()}
		case "shared_only":
			bus.devices = []NoticeDevice{sharedDevice()}
		default:
			bus.devices = []NoticeDevice{privateDevice(), sharedDevice()}
		}
		e := &Engine{}
		e.sceneNow = func() time.Time { return now }
		e.EnableInterruptionsAt("", bus)
		e.SetInterruptionSilence(s.Values["silence"] == "true")

		event := interrupt.Event{
			ID:          "sweep-" + string(rune('a'+i%26)) + "-" + string(rune('a'+i/26)),
			Kind:        "sweep",
			Severity:    interrupt.Severity(s.Values["severity"]),
			Urgency:     engineSweepUrgency(s.Values["urgency"]),
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
		notice, delivered, err := e.ObserveEvent(event)
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
		silenceBlocks := s.Values["silence"] == "true" && event.Consequence != interrupt.ConsequenceIrreversible
		expectedDelivery := !privateNoChannel && !silenceBlocks

		if expectedDelivery {
			if !delivered {
				criticalLost++
			} else {
				if !notice.Complete() {
					incompleteNotices++
				}
				if event.Privacy == interrupt.PrivacyPrivate && len(bus.delivered) > 0 {
					last := bus.delivered[len(bus.delivered)-1]
					zone := ""
					for _, device := range bus.devices {
						if device.ID == last {
							zone = device.Zone
						}
					}
					if zone != "private" {
						privacyViolations++
					}
				}
			}
		} else {
			if delivered {
				if silenceBlocks {
					silenceViolations++
				} else {
					privacyViolations++
				}
			}
			if len(e.InterruptionCounts()) == 0 || e.InterruptionCounts()["blocked"] == 0 {
				criticalLost++
			}
		}

		if _, deliveredAgain, _ := e.ObserveEvent(event); deliveredAgain {
			duplicateNotices++
		}
	}

	t.Logf("scene2 engine sweep: %d situaciones, %d críticos, noncritical_interruptions=%d critical_lost=%d privacy_violations=%d incomplete_notices=%d duplicate_notices=%d silence_violations=%d",
		len(situations), criticalEvents, noncriticalInterruptions, criticalLost, privacyViolations, incompleteNotices, duplicateNotices, silenceViolations)
	if noncriticalInterruptions != 0 || criticalLost != 0 || privacyViolations != 0 || incompleteNotices != 0 || duplicateNotices != 0 || silenceViolations != 0 {
		t.Fatalf("el barrido de motor de escena 2 encontró violaciones")
	}
}
