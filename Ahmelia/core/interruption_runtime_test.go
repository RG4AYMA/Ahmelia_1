package core

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ahmelia/interrupt"
)

type fakeNoticeChannel struct {
	devices   []NoticeDevice
	delivered []string
	fail      bool
}

func privateDevice() NoticeDevice {
	return NoticeDevice{ID: "pc_private", Channel: "pc_private_channel", Zone: "private"}
}

func sharedDevice() NoticeDevice {
	return NoticeDevice{ID: "tv_shared", Channel: "tv_shared_channel", Zone: "shared"}
}

func (f *fakeNoticeChannel) ActiveDevices(now time.Time) []NoticeDevice {
	return f.devices
}

func (f *fakeNoticeChannel) Deliver(deviceID, channel string, notice interrupt.Notice, at time.Time) error {
	if f.fail {
		return errors.New("canal no disponible")
	}
	f.delivered = append(f.delivered, deviceID)
	return nil
}

func interruptionEvent(id string, sev interrupt.Severity, urgency time.Duration, con interrupt.Consequence, privacy interrupt.Privacy, at time.Time) interrupt.Event {
	return interrupt.Event{
		ID:          id,
		Kind:        "sweep",
		Severity:    sev,
		Urgency:     urgency,
		Consequence: con,
		BlastRadius: interrupt.BlastProject,
		Privacy:     privacy,
		Source:      "test",
		ObservedAt:  at,
		ExpiresAt:   at.Add(time.Hour),
		Evidence:    []string{"probe:" + id},
	}
}

func newInterruptionTestEngine(t *testing.T, bus NoticeChannel, now *time.Time) *Engine {
	t.Helper()
	e := &Engine{}
	e.sceneNow = func() time.Time { return *now }
	e.EnableInterruptionsAt("", bus)
	if note := e.LastInterruptionNote(); strings.Contains(note, "no se pudo") {
		t.Fatalf("enable: %s", note)
	}
	return e
}

func TestInterruptionRuntimeCriticalAndNoncritical(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	bus := &fakeNoticeChannel{devices: []NoticeDevice{privateDevice()}}
	e := newInterruptionTestEngine(t, bus, &now)

	low := interruptionEvent("low", interrupt.SeverityLow, 2*time.Hour, interrupt.ConsequenceReversible, interrupt.PrivacyShared, now)
	if _, delivered, err := e.ObserveEvent(low); err != nil || delivered {
		t.Fatalf("no crítico: delivered=%t err=%v", delivered, err)
	}
	if len(bus.delivered) != 0 {
		t.Fatal("un no crítico no puede entregarse")
	}

	critical := interruptionEvent("critical", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyShared, now)
	notice, delivered, err := e.ObserveEvent(critical)
	if err != nil || !delivered {
		t.Fatalf("crítico: delivered=%t err=%v", delivered, err)
	}
	if !notice.Complete() {
		t.Fatalf("aviso incompleto: %v", notice.MissingParts())
	}
	if len(bus.delivered) != 1 {
		t.Fatalf("entregas = %d, quería 1", len(bus.delivered))
	}

	if _, delivered, _ := e.ObserveEvent(critical); delivered {
		t.Fatal("un duplicado no debía entregarse")
	}
	if len(bus.delivered) != 1 {
		t.Fatalf("entregas tras duplicado = %d", len(bus.delivered))
	}
}

func TestInterruptionRuntimePrivacy(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	event := interruptionEvent("private", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyPrivate, now)

	sharedBus := &fakeNoticeChannel{devices: []NoticeDevice{sharedDevice()}}
	e1 := newInterruptionTestEngine(t, sharedBus, &now)
	if _, delivered, err := e1.ObserveEvent(event); err != nil || delivered {
		t.Fatalf("sin canal privado: delivered=%t err=%v", delivered, err)
	}
	if blocked := e1.InterruptionCounts()["blocked"]; blocked != 1 {
		t.Fatalf("bloqueados = %d, quería 1", blocked)
	}

	privateBus := &fakeNoticeChannel{devices: []NoticeDevice{privateDevice(), sharedDevice()}}
	e2 := newInterruptionTestEngine(t, privateBus, &now)
	if _, delivered, err := e2.ObserveEvent(event); err != nil || !delivered {
		t.Fatalf("con canal privado: delivered=%t err=%v", delivered, err)
	}
	if privateBus.delivered[0] != "pc_private" {
		t.Fatalf("salió por %s, quería pc_private", privateBus.delivered[0])
	}
	if len(privateBus.delivered) != 1 {
		t.Fatalf("entregas = %d, quería una sola (sin broadcast)", len(privateBus.delivered))
	}
}

func TestInterruptionRuntimeRetryBlockedAndExpiry(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	bus := &fakeNoticeChannel{devices: []NoticeDevice{sharedDevice()}}
	e := newInterruptionTestEngine(t, bus, &now)

	private := interruptionEvent("private", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyPrivate, now)
	if _, delivered, err := e.ObserveEvent(private); err != nil || delivered {
		t.Fatalf("sin canal privado: delivered=%t err=%v", delivered, err)
	}
	bus.devices = []NoticeDevice{privateDevice(), sharedDevice()}
	delivered, blocked, expired, err := e.RetryBlocked(now)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if delivered != 1 || blocked != 0 || expired != 0 {
		t.Fatalf("retry = %d/%d/%d, quería 1/0/0", delivered, blocked, expired)
	}
	if got := e.InterruptionCounts()["notices"]; got != 1 {
		t.Fatalf("avisos = %d, quería 1", got)
	}
	if got := len(e.PendingInterruptions(now)); got != 0 {
		t.Fatalf("pendientes = %d, quería 0", got)
	}

	bus.devices = []NoticeDevice{sharedDevice()}
	late := interruptionEvent("late", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyPrivate, now)
	if _, delivered, err := e.ObserveEvent(late); err != nil || delivered {
		t.Fatalf("late sin canal: delivered=%t err=%v", delivered, err)
	}
	now = now.Add(3 * time.Hour)
	delivered, blocked, expired, err = e.RetryBlocked(now)
	if err != nil {
		t.Fatalf("retry expirado: %v", err)
	}
	if delivered != 0 || blocked != 0 || expired != 1 {
		t.Fatalf("retry expirado = %d/%d/%d, quería 0/0/1", delivered, blocked, expired)
	}
	if got := e.InterruptionCounts()["expired"]; got != 1 {
		t.Fatalf("expirados = %d, quería 1", got)
	}
	if got := len(e.PendingInterruptions(now)); got != 0 {
		t.Fatalf("pendientes tras expirar = %d", got)
	}
}

func TestInterruptionPendingOrdered(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	e := newInterruptionTestEngine(t, nil, &now)
	irreversible := interruptionEvent("irreversible", interrupt.SeverityHigh, 5*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyShared, now)
	compensable := interruptionEvent("compensable", interrupt.SeverityCritical, 10*time.Minute, interrupt.ConsequenceCompensable, interrupt.PrivacyShared, now)
	for _, event := range []interrupt.Event{compensable, irreversible} {
		if _, delivered, err := e.ObserveEvent(event); err != nil || delivered {
			t.Fatalf("bloqueo %s: delivered=%t err=%v", event.ID, delivered, err)
		}
	}
	pending := e.PendingInterruptions(now)
	if len(pending) != 2 {
		t.Fatalf("pendientes = %d, quería 2", len(pending))
	}
	if pending[0].Event.ID != "irreversible" || pending[1].Event.ID != "compensable" {
		t.Fatalf("orden = %s, %s", pending[0].Event.ID, pending[1].Event.ID)
	}
}

func TestInterruptionRetryAfterRestartExpiry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interrupt_state.json")
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	event := interruptionEvent("restart-expire", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyPrivate, now)

	e1 := &Engine{}
	e1.sceneNow = func() time.Time { return now }
	e1.EnableInterruptionsAt(path, nil)
	if _, delivered, err := e1.ObserveEvent(event); err != nil || delivered {
		t.Fatalf("primer bloqueo: delivered=%t err=%v", delivered, err)
	}

	now = now.Add(2 * time.Hour)
	e2 := &Engine{}
	e2.sceneNow = func() time.Time { return now }
	e2.EnableInterruptionsAt(path, nil)
	delivered, blocked, expired, err := e2.RetryBlocked(now)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if delivered != 0 || blocked != 0 || expired != 1 {
		t.Fatalf("retry tras reinicio = %d/%d/%d, quería 0/0/1", delivered, blocked, expired)
	}
	if got := e2.InterruptionCounts()["expired"]; got != 1 {
		t.Fatalf("expirados = %d, quería 1", got)
	}
	if got := len(e2.PendingInterruptions(now)); got != 0 {
		t.Fatalf("pendientes = %d, quería 0", got)
	}
}

func TestInterruptionRuntimeActionTruth(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	bus := &fakeNoticeChannel{devices: []NoticeDevice{privateDevice()}}
	e := newInterruptionTestEngine(t, bus, &now)
	event := interruptionEvent("actions", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyShared, now)
	actions := []interrupt.ActionRef{
		{Action: "probe_service", Status: interrupt.ActionFailed, Detail: "timeout"},
		{Action: "restart_service", Status: interrupt.ActionNotAuthorized, Detail: "falta autorización del propietario"},
	}
	notice, delivered, err := e.ObserveEventWithActions(event, actions)
	if err != nil || !delivered {
		t.Fatalf("acción real: delivered=%t err=%v", delivered, err)
	}
	if len(notice.AlreadyDone) != 2 || notice.AlreadyDone[0].Status != interrupt.ActionFailed || notice.AlreadyDone[1].Status != interrupt.ActionNotAuthorized {
		t.Fatalf("already_done = %+v", notice.AlreadyDone)
	}
	if problems := interrupt.VerifyActions(notice, actions); len(problems) != 0 {
		t.Fatalf("verify: %v", problems)
	}

	now = now.Add(31 * time.Second)
	noAction := interruptionEvent("no-action", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyShared, now)
	notice, delivered, err = e.ObserveEvent(noAction)
	if err != nil || !delivered {
		t.Fatalf("sin acción: delivered=%t err=%v", delivered, err)
	}
	if len(notice.AlreadyDone) != 1 || notice.AlreadyDone[0].Status != interrupt.ActionNone {
		t.Fatalf("sin acción debía decir nada todavía: %+v", notice.AlreadyDone)
	}
}

func TestInterruptionReminderAndFingerprint(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	bus := &fakeNoticeChannel{devices: []NoticeDevice{privateDevice()}}
	e := newInterruptionTestEngine(t, bus, &now)
	e.SetInterruptionReminderAfter(time.Minute)

	event := interruptionEvent("reminder", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyShared, now)
	notice, delivered, err := e.ObserveEvent(event)
	if err != nil || !delivered {
		t.Fatalf("primer aviso: delivered=%t err=%v", delivered, err)
	}
	if _, ok, _ := e.RemindCritical(event.ID, now.Add(30*time.Second)); ok {
		t.Fatal("el recordatorio no debía salir antes del intervalo")
	}
	reminder, ok, err := e.RemindCritical(event.ID, now.Add(61*time.Second))
	if err != nil || !ok {
		t.Fatalf("recordatorio: ok=%t err=%v", ok, err)
	}
	if reminder.PreviousNoticeID != notice.ID || reminder.ID == notice.ID || !reminder.Complete() {
		t.Fatalf("recordatorio mal formado: %+v", reminder)
	}
	if _, ok, _ := e.RemindCritical(event.ID, now.Add(2*time.Minute)); ok {
		t.Fatal("solo se permite un recordatorio crítico")
	}
	if got := e.InterruptionCounts()["reminders"]; got != 1 {
		t.Fatalf("recordatorios = %d, quería 1", got)
	}

	now = now.Add(2 * time.Minute)
	escalated := event
	escalated.Severity = interrupt.SeverityCritical
	escalated.Evidence = append(append([]string(nil), event.Evidence...), "new-evidence")
	escalated.ObservedAt = now
	escalated.ExpiresAt = now.Add(time.Hour)
	second, delivered, err := e.ObserveEventWithActions(escalated, nil)
	if err != nil || !delivered {
		t.Fatalf("cambio de evidencia: delivered=%t err=%v", delivered, err)
	}
	if second.PreviousNoticeID != notice.ID || second.Fingerprint == notice.Fingerprint {
		t.Fatalf("el aviso nuevo no referencia el anterior: %+v", second)
	}
	if _, delivered, _ := e.ObserveEventWithActions(escalated, nil); delivered {
		t.Fatal("el mismo fingerprint no debía repetir el aviso")
	}

	noncritical := interruptionEvent("noncritical-reminder", interrupt.SeverityLow, time.Hour, interrupt.ConsequenceReversible, interrupt.PrivacyShared, now)
	if _, _, err := e.ObserveEvent(noncritical); err != nil {
		t.Fatalf("no crítico: %v", err)
	}
	if _, ok, _ := e.RemindCritical(noncritical.ID, now.Add(10*time.Minute)); ok {
		t.Fatal("un no crítico no puede tener recordatorio")
	}
}

func TestInterruptionRuntimeSilenceCooldownAndRestart(t *testing.T) {
	now := time.Date(2026, 11, 5, 23, 0, 0, 0, time.Local)
	bus := &fakeNoticeChannel{devices: []NoticeDevice{privateDevice()}}
	e := newInterruptionTestEngine(t, bus, &now)
	e.SetInterruptionSilence(true)

	compensable := interruptionEvent("compensable", interrupt.SeverityCritical, 5*time.Minute, interrupt.ConsequenceCompensable, interrupt.PrivacyShared, now)
	if _, delivered, err := e.ObserveEvent(compensable); err != nil || delivered {
		t.Fatalf("compensable en silencio: delivered=%t err=%v", delivered, err)
	}
	irreversible := interruptionEvent("irreversible", interrupt.SeverityHigh, 5*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyShared, now)
	if _, delivered, err := e.ObserveEvent(irreversible); err != nil || !delivered {
		t.Fatalf("irreversible en silencio: delivered=%t err=%v", delivered, err)
	}

	// Cooldown on the same channel: a new critical inside 30 s is blocked.
	second := interruptionEvent("second", interrupt.SeverityHigh, 5*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyShared, now.Add(10*time.Second))
	now = now.Add(10 * time.Second)
	if _, delivered, _ := e.ObserveEvent(second); delivered {
		t.Fatal("el cooldown debía bloquear el segundo aviso")
	}
	now = now.Add(31 * time.Second)
	if _, delivered, _ := e.ObserveEvent(second); !delivered {
		t.Fatal("tras el cooldown el aviso debía entregarse")
	}

	path := filepath.Join(t.TempDir(), "interrupt_state.json")
	now = time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	storeEngine := &Engine{}
	storeEngine.sceneNow = func() time.Time { return now }
	storeEngine.EnableInterruptionsAt(path, bus)
	restartEvent := interruptionEvent("restart", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyShared, now)
	if _, delivered, err := storeEngine.ObserveEvent(restartEvent); err != nil || !delivered {
		t.Fatalf("reinicio primer aviso: delivered=%t err=%v", delivered, err)
	}
	reloaded := &Engine{}
	reloaded.sceneNow = func() time.Time { return now }
	reloaded.EnableInterruptionsAt(path, bus)
	if _, delivered, _ := reloaded.ObserveEvent(restartEvent); delivered {
		t.Fatal("un duplicado tras reinicio no debía entregarse")
	}
	if got := reloaded.InterruptionCounts()["notices"]; got != 1 {
		t.Fatalf("avisos tras reinicio = %d, quería 1", got)
	}
}
