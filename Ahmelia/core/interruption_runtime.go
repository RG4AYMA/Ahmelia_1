package core

// interruption_runtime.go — ESCENA 2 EN EL MOTOR.
//
// El bus de canales es una frontera inyectable: el motor puntúa, compone y
// entrega; el camino crítico es determinista y no consulta al modelo. Un canal
// ausente o un cooldown dejan el crítico bloqueado y reintentable, nunca
// descartado en silencio.

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"ahmelia/interrupt"
)

// NoticeDevice is one active delivery endpoint. The engine filters by privacy
// and chooses exactly one; it never broadcasts.
type NoticeDevice struct {
	ID      string
	Channel string
	Zone    string
}

// NoticeChannel is the device boundary for critical notices. ActiveDevices
// reports presence at now; the engine enforces privacy and single-device
// delivery. Deliver must report a real failure so the runtime can block and
// retry.
type NoticeChannel interface {
	ActiveDevices(now time.Time) []NoticeDevice
	Deliver(deviceID, channel string, notice interrupt.Notice, at time.Time) error
}

var errInterruptionsDisabled = errors.New("interruptions: runtime no habilitado")

// EnableInterruptionsAt turns on the event runtime with an injectable channel.
func (e *Engine) EnableInterruptionsAt(path string, bus NoticeChannel) {
	if e == nil {
		return
	}
	store, err := interrupt.Open(path)
	e.deliberationMu.Lock()
	defer e.deliberationMu.Unlock()
	if err != nil {
		e.interruptionStore = nil
		e.interruptionOn = false
		e.lastInterruptionNote = "no se pudo cargar el runtime de interrupciones: " + err.Error()
		return
	}
	e.interruptionStore = store
	e.interruptionBus = bus
	e.interruptionOn = true
	if e.interruptionCooldown <= 0 {
		e.interruptionCooldown = 30 * time.Second
	}
	if e.interruptionReminderAfter <= 0 {
		e.interruptionReminderAfter = 5 * time.Minute
	}
	e.lastInterruptionNote = ""
	if bus == nil {
		e.lastInterruptionNote = "sin canal de avisos: los críticos quedarán bloqueados y reintentables"
	}
}

// SetInterruptionCooldown changes the per-channel cooldown. It does not affect
// already delivered notices.
func (e *Engine) SetInterruptionCooldown(window time.Duration) {
	if e == nil {
		return
	}
	e.deliberationMu.Lock()
	if window <= 0 {
		window = 30 * time.Second
	}
	e.interruptionCooldown = window
	e.deliberationMu.Unlock()
}

// SetInterruptionReminderAfter sets the interval before the single critical
// reminder. A second reminder is never sent.
func (e *Engine) SetInterruptionReminderAfter(after time.Duration) {
	if e == nil {
		return
	}
	e.deliberationMu.Lock()
	if after <= 0 {
		after = 5 * time.Minute
	}
	e.interruptionReminderAfter = after
	e.deliberationMu.Unlock()
}

// SetInterruptionSilence defers compensable criticals while keeping
// irreversible ones deliverable.
func (e *Engine) SetInterruptionSilence(silent bool) {
	if e == nil {
		return
	}
	e.deliberationMu.Lock()
	e.interruptionSilence = silent
	e.deliberationMu.Unlock()
}

func (e *Engine) interruptionSnapshot() (*interrupt.Store, NoticeChannel, bool, time.Duration, bool) {
	if e == nil {
		return nil, nil, false, 0, false
	}
	e.deliberationMu.Lock()
	defer e.deliberationMu.Unlock()
	enabled := e.interruptionOn && e.interruptionStore != nil
	return e.interruptionStore, e.interruptionBus, enabled, e.interruptionCooldown, e.interruptionSilence
}

// ObserveEvent scores one event and either records it in the digest/escalated
// list or delivers a critical notice through the injected channel.
func (e *Engine) ObserveEvent(event interrupt.Event) (interrupt.Notice, bool, error) {
	return e.ObserveEventWithActions(event, nil)
}

// ObserveEventWithActions is the same path with the real action log. The
// notice copies those actions; the runtime verifies that the notice did not
// invent or omit one before delivering.
func (e *Engine) ObserveEventWithActions(event interrupt.Event, actions []interrupt.ActionRef) (interrupt.Notice, bool, error) {
	store, bus, enabled, cooldown, silence := e.interruptionSnapshot()
	if !enabled {
		return interrupt.Notice{}, false, errInterruptionsDisabled
	}
	score, err := interrupt.Assess(event)
	if err != nil {
		return interrupt.Notice{}, false, err
	}
	if !score.Critical {
		if score.Escalated {
			return interrupt.Notice{}, false, store.AddEscalated(event)
		}
		return interrupt.Notice{}, false, store.AddDigest(event)
	}
	previousID := ""
	if notice, seen := store.Delivered(event.ID); seen {
		if notice.Fingerprint == event.Fingerprint() {
			return notice, false, nil
		}
		previousID = notice.ID
	}
	return e.attemptCritical(event, e.nowTime(), store, bus, cooldown, silence, actions, previousID)
}

func selectNoticeDevice(event interrupt.Event, bus NoticeChannel, now time.Time) (NoticeDevice, bool) {
	if bus == nil {
		return NoticeDevice{}, false
	}
	for _, device := range bus.ActiveDevices(now) {
		if device.ID == "" || device.Channel == "" {
			continue
		}
		if event.Privacy == interrupt.PrivacyPrivate && device.Zone != "private" {
			continue
		}
		return device, true
	}
	return NoticeDevice{}, false
}

func (e *Engine) attemptCritical(event interrupt.Event, now time.Time, store *interrupt.Store, bus NoticeChannel, cooldown time.Duration, silence bool, actions []interrupt.ActionRef, previousID string) (interrupt.Notice, bool, error) {
	if silence && event.Consequence != interrupt.ConsequenceIrreversible {
		return interrupt.Notice{}, false, store.Block(event, "silence", now)
	}
	if bus == nil {
		return interrupt.Notice{}, false, store.Block(event, "no_channel", now)
	}
	device, ok := selectNoticeDevice(event, bus, now)
	if !ok {
		return interrupt.Notice{}, false, store.Block(event, "no_channel", now)
	}
	channel := device.Channel
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	if store.CooldownActive(channel, now, cooldown) {
		return interrupt.Notice{}, false, store.Block(event, "cooldown", now)
	}
	noticeID := "notice-" + event.ID
	if previousID != "" {
		noticeID = fmt.Sprintf("notice-%s-%d", event.ID, now.UnixNano())
	}
	notice, err := interrupt.Compose(interrupt.ComposeRequest{
		Event:            event,
		NoticeID:         noticeID,
		AlreadyDone:      actions,
		Confidence:       0.9,
		Channel:          channel,
		PreviousNoticeID: previousID,
	}, now)
	if err != nil {
		return interrupt.Notice{}, false, err
	}
	notice.DeviceID = device.ID
	if problems := interrupt.VerifyActions(notice, actions); len(problems) != 0 {
		return interrupt.Notice{}, false, fmt.Errorf("interrupt: acciones del aviso no coinciden: %s", strings.Join(problems, ","))
	}
	if err := bus.Deliver(device.ID, channel, notice, now); err != nil {
		return interrupt.Notice{}, false, store.Block(event, "delivery_failed", now)
	}
	if err := store.RecordDelivery(notice, now); err != nil {
		return interrupt.Notice{}, false, err
	}
	return notice, true, nil
}

// RemindCritical sends the single critical reminder after the configured
// interval. It never sends a second reminder and never reminds a non-critical
// event (those never have a delivered notice).
func (e *Engine) RemindCritical(eventID string, now time.Time) (interrupt.Notice, bool, error) {
	store, bus, enabled, _, _ := e.interruptionSnapshot()
	if !enabled {
		return interrupt.Notice{}, false, errInterruptionsDisabled
	}
	if bus == nil {
		return interrupt.Notice{}, false, errors.New("interruptions: sin canal de aviso")
	}
	notice, ok := store.Delivered(eventID)
	if !ok {
		return interrupt.Notice{}, false, fmt.Errorf("interruptions: sin aviso previo para %s", eventID)
	}
	if store.ReminderSent(eventID) {
		return interrupt.Notice{}, false, nil
	}
	e.deliberationMu.Lock()
	after := e.interruptionReminderAfter
	e.deliberationMu.Unlock()
	if after <= 0 {
		after = 5 * time.Minute
	}
	if notice.DeliveredAt.IsZero() || now.Before(notice.DeliveredAt.Add(after)) {
		return interrupt.Notice{}, false, nil
	}
	if !notice.ExpiresAt.IsZero() && now.After(notice.ExpiresAt) {
		return interrupt.Notice{}, false, nil
	}
	if notice.DeviceID == "" || notice.Channel == "" {
		return interrupt.Notice{}, false, errors.New("interruptions: aviso sin dispositivo/canal")
	}
	reminder := notice
	reminder.ID = notice.ID + "-reminder"
	reminder.PreviousNoticeID = notice.ID
	reminder.DeliveredAt = now
	if err := bus.Deliver(notice.DeviceID, notice.Channel, reminder, now); err != nil {
		return interrupt.Notice{}, false, err
	}
	if err := store.MarkReminder(eventID, now); err != nil {
		return interrupt.Notice{}, false, err
	}
	return reminder, true, nil
}

// RetryBlocked re-attempts every pending critical in priority order. Expired// events are recorded as expired, not discarded; events that still cannot be
// delivered stay blocked with their history.
func (e *Engine) RetryBlocked(now time.Time) (delivered int, blocked int, expired int, err error) {
	store, bus, enabled, cooldown, silence := e.interruptionSnapshot()
	if !enabled {
		return 0, 0, 0, errInterruptionsDisabled
	}
	expiredRecords, err := store.Expire(now)
	if err != nil {
		return 0, 0, 0, err
	}
	for _, pending := range store.PendingOrdered(now) {
		score, assessErr := interrupt.Assess(pending.Event)
		if assessErr != nil || !score.Critical {
			continue
		}
		_, wasDelivered, attemptErr := e.attemptCritical(pending.Event, now, store, bus, cooldown, silence, nil, "")
		if attemptErr != nil {
			return delivered, blocked, len(expiredRecords), attemptErr
		}
		if wasDelivered {
			delivered++
		} else {
			blocked++
		}
	}
	return delivered, blocked, len(expiredRecords), nil
}

// PendingInterruptions returns the retryable criticals in priority order.
func (e *Engine) PendingInterruptions(now time.Time) []interrupt.Blocked {
	store, _, enabled, _, _ := e.interruptionSnapshot()
	if !enabled {
		return nil
	}
	return store.PendingOrdered(now)
}

// SweepInterruptions records the expiry of blocked events. It returns the newly
// expired records; nothing is deleted without a trace.
func (e *Engine) SweepInterruptions(now time.Time) ([]interrupt.Expired, error) {
	store := func() *interrupt.Store {
		if e == nil {
			return nil
		}
		e.deliberationMu.Lock()
		defer e.deliberationMu.Unlock()
		if !e.interruptionOn {
			return nil
		}
		return e.interruptionStore
	}()
	if store == nil {
		return nil, nil
	}
	return store.Expire(now)
}

// LastInterruptionNote reports load or channel warnings.
func (e *Engine) LastInterruptionNote() string {
	if e == nil {
		return ""
	}
	e.deliberationMu.Lock()
	defer e.deliberationMu.Unlock()
	return e.lastInterruptionNote
}

// InterruptionCounts summarizes the runtime for diagnostics.
func (e *Engine) InterruptionCounts() map[string]int {
	store, _, enabled, _, _ := e.interruptionSnapshot()
	if !enabled {
		return nil
	}
	return map[string]int{
		"notices":   len(store.Notices()),
		"blocked":   len(store.BlockedList()),
		"digest":    len(store.Digest()),
		"escalated": len(store.Escalated()),
		"reminders": store.ReminderCount(),
		"expired":   len(store.History()),
	}
}
