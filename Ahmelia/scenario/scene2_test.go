package scenario

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ahmelia/consult"
	"ahmelia/interrupt"
)

// scene2Delivery is one observed interruption.
type scene2Delivery struct {
	EventID    string
	DeviceID   string
	ObservedAt time.Time
	At         time.Time
}

// scene2Harness is the deterministic double for the critical-interruption
// scene. It decides nothing by prose: the score comes from interrupt.Assess,
// the notice from interrupt.Compose, the durable state from interrupt.Store and
// delivery from FakeDeviceBus.
type scene2Harness struct {
	clock      *Clock
	bus        *FakeDeviceBus
	store      *interrupt.Store
	deliveries []scene2Delivery
	duplicates []string
	actionLog  []interrupt.ActionRef
	silence    bool
	cooldown   time.Duration
}

func newScene2Harness(clock *Clock, bus *FakeDeviceBus, store *interrupt.Store) *scene2Harness {
	return &scene2Harness{clock: clock, bus: bus, store: store, cooldown: 30 * time.Second}
}

func (h *scene2Harness) chooseChannel(e interrupt.Event) string {
	wantPrivate := e.Privacy == interrupt.PrivacyPrivate
	for _, id := range []string{"pc_private", "tv_shared", "speaker_public"} {
		device, ok := h.bus.Device(id)
		if !ok || !device.Online {
			continue
		}
		if wantPrivate && device.Zone != "private" {
			continue
		}
		return id
	}
	return ""
}

// observe runs one event through the deterministic path. It uses the same
// candidate and validation policy as the model path, with an abstaining
// proposal: the harness fallback decides.
func (h *scene2Harness) observe(e interrupt.Event, options []interrupt.Option, decision string) (interrupt.Notice, bool, error) {
	score, err := interrupt.Assess(e)
	if err != nil {
		return interrupt.Notice{}, false, err
	}
	if notice, seen := h.store.Delivered(e.ID); seen {
		h.duplicates = append(h.duplicates, e.ID)
		return notice, false, nil
	}
	channel := h.chooseChannel(e)
	cooldown := channel != "" && h.store.CooldownActive(channel, h.clock.Now(), h.cooldown)
	candidates := Scene2Candidates(e, score, channel, cooldown, h.silence)
	d := consult.Decide(
		consult.Proposal{Abstain: true},
		candidates,
		Scene2Allow(e, score, channel, cooldown, h.silence),
		Scene2Fallback(e, score, channel, cooldown, h.silence),
	)
	return h.applyCandidate(e, score, d, options, decision)
}

// scene2ConsultOutcome keeps the model contribution and the harness verdict
// together so a test can assert both.
type scene2ConsultOutcome struct {
	Decision  consult.Decision
	Proposal  consult.Proposal
	Notice    interrupt.Notice
	Delivered bool
}

// observeWithConsult runs one event through the model consultation and the
// harness gate. A model error does not stop the deterministic path: the
// fallback candidate is applied and the error is returned for the trace.
func (h *scene2Harness) observeWithConsult(ctx context.Context, e interrupt.Event, client consult.Client, options []interrupt.Option, decision string) (scene2ConsultOutcome, error) {
	score, err := interrupt.Assess(e)
	if err != nil {
		return scene2ConsultOutcome{}, err
	}
	if notice, seen := h.store.Delivered(e.ID); seen {
		h.duplicates = append(h.duplicates, e.ID)
		return scene2ConsultOutcome{Notice: notice}, nil
	}

	channel := h.chooseChannel(e)
	cooldown := channel != "" && h.store.CooldownActive(channel, h.clock.Now(), h.cooldown)
	candidates := Scene2Candidates(e, score, channel, cooldown, h.silence)
	ownerState := "busy"
	restriction := ""
	if h.silence {
		ownerState = "resting"
		restriction = "silencio activo"
	}
	req := consult.Request{
		Scene:          "scene2_critical_interruptions",
		Input:          e.Kind,
		HarnessReading: score.String(),
		OwnerState:     ownerState,
		Restriction:    restriction,
		Candidates:     candidates,
		Evidence: []consult.Evidence{
			{ID: "event", Text: fmt.Sprintf("%s severity=%s urgency=%s consequence=%s privacy=%s",
				e.Kind, e.Severity, e.Urgency, e.Consequence, e.Privacy)},
			{ID: "score", Text: score.String()},
		},
	}
	proposal, consultErr := consult.Consult(ctx, client, req)
	d := consult.Decide(
		proposal,
		candidates,
		Scene2Allow(e, score, channel, cooldown, h.silence),
		Scene2Fallback(e, score, channel, cooldown, h.silence),
	)
	notice, delivered, applyErr := h.applyCandidate(e, score, d, options, decision)
	return scene2ConsultOutcome{
		Decision: d, Proposal: proposal, Notice: notice, Delivered: delivered,
	}, errors.Join(consultErr, applyErr)
}

func (h *scene2Harness) applyCandidate(e interrupt.Event, score interrupt.Score, d consult.Decision, options []interrupt.Option, decision string) (interrupt.Notice, bool, error) {
	switch d.Candidate.ID {
	case Scene2Interrupt:
		channel := h.chooseChannel(e)
		if channel == "" {
			return interrupt.Notice{}, false, h.store.Block(e, "no_channel", h.clock.Now())
		}
		return h.deliver(e, options, decision, channel)
	case Scene2Digest:
		if score.Escalated {
			// An irreversible event below the interrupt threshold is never
			// dropped: it is escalated for review.
			return interrupt.Notice{}, false, h.store.AddEscalated(e)
		}
		return interrupt.Notice{}, false, h.store.AddDigest(e)
	case Scene2Block:
		reason := "blocked"
		switch {
		case h.silence && e.Consequence != interrupt.ConsequenceIrreversible:
			reason = "silence"
		case h.chooseChannel(e) == "":
			reason = "no_channel"
		case h.store.CooldownActive(h.chooseChannel(e), h.clock.Now(), h.cooldown):
			reason = "cooldown_active"
		}
		return interrupt.Notice{}, false, h.store.Block(e, reason, h.clock.Now())
	default:
		return interrupt.Notice{}, false, nil
	}
}

func (h *scene2Harness) deliver(e interrupt.Event, options []interrupt.Option, decision, channel string) (interrupt.Notice, bool, error) {
	notice, err := interrupt.Compose(interrupt.ComposeRequest{
		Event:          e,
		NoticeID:       "notice-" + e.ID,
		AlreadyDone:    h.actionLog,
		Options:        options,
		DecisionNeeded: decision,
		Confidence:     0.9,
		Channel:        channel,
	}, h.clock.Now())
	if err != nil {
		return interrupt.Notice{}, false, err
	}
	if _, err := h.bus.Execute(channel, "notify_critical", h.clock.Now()); err != nil {
		return interrupt.Notice{}, false, h.store.Block(e, "delivery_failed", h.clock.Now())
	}
	if err := h.store.RecordDelivery(notice, h.clock.Now()); err != nil {
		return interrupt.Notice{}, false, err
	}
	h.deliveries = append(h.deliveries, scene2Delivery{
		EventID: e.ID, DeviceID: channel, ObservedAt: e.ObservedAt, At: h.clock.Now(),
	})
	return notice, true, nil
}

func scene2Event(id, kind string, sev interrupt.Severity, urgency time.Duration, con interrupt.Consequence, privacy interrupt.Privacy, at time.Time) interrupt.Event {
	return interrupt.Event{
		ID:          id,
		Kind:        kind,
		Severity:    sev,
		Urgency:     urgency,
		Consequence: con,
		BlastRadius: interrupt.BlastProject,
		Privacy:     privacy,
		Source:      "monitor",
		ObservedAt:  at,
		ExpiresAt:   at.Add(time.Hour),
		Evidence:    []string{"probe:" + kind},
	}
}

func TestScene2CriticalInterruptions(t *testing.T) {
	loc := time.FixedZone("COT", -5*3600)
	start := time.Date(2026, 11, 5, 9, 0, 0, 0, loc)
	clock := NewClock(start, loc)
	bus := NewFakeDeviceBus(
		FakeDevice{ID: "pc_private", Kind: "desktop", Zone: "private", Online: true},
		FakeDevice{ID: "tv_shared", Kind: "tv", Zone: "shared", Online: true},
		FakeDevice{ID: "speaker_public", Kind: "speaker", Zone: "public", Online: true},
	)
	store, err := interrupt.Open(filepath.Join(t.TempDir(), "interrupt_state.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	h := newScene2Harness(clock, bus, store)

	// 09:00–11:00 — ordinary operational noise: digest, zero interruptions.
	for i, spec := range []struct {
		hour int
		id   string
		kind string
		sev  interrupt.Severity
		con  interrupt.Consequence
	}{
		{9, "disk85", "disk_pressure", interrupt.SeverityLow, interrupt.ConsequenceReversible},
		{10, "slow_svc", "service_slow", interrupt.SeverityMedium, interrupt.ConsequenceCompensable},
		{11, "backup_late", "backup_delayed", interrupt.SeverityMedium, interrupt.ConsequenceCompensable},
	} {
		clock.At(time.Date(2026, 11, 5, spec.hour, 0, 0, 0, loc))
		if _, delivered, err := h.observe(scene2Event(spec.id, spec.kind, spec.sev, 2*time.Hour, spec.con, interrupt.PrivacyShared, clock.Now()), nil, ""); err != nil || delivered {
			t.Fatalf("evento no crítico %d: delivered=%t err=%v", i, delivered, err)
		}
	}
	if len(store.Digest()) != 3 {
		t.Fatalf("digest = %d, quería 3", len(store.Digest()))
	}
	if len(h.deliveries) != 0 {
		t.Fatalf("interrupciones no críticas = %d, quería 0", len(h.deliveries))
	}

	// 12:00 — irreversible database loss in 10 minutes. It must interrupt with
	// a complete notice, and the mitigation is a real executed action.
	clock.At(time.Date(2026, 11, 5, 12, 0, 0, 0, loc))
	dbEvent := scene2Event("db_down", "database_down", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyShared, clock.Now())
	h.actionLog = []interrupt.ActionRef{{
		Action: "probe_database", Status: interrupt.ActionExecuted, Detail: "connection refused", At: clock.Now(),
	}}
	notice, delivered, err := h.observe(dbEvent,
		[]interrupt.Option{{Label: "Reiniciar la base", Consequence: "Vuelve en dos minutos", Safe: true}},
		"¿Reinicio la base ahora?")
	if err != nil || !delivered {
		t.Fatalf("crítico 12:00: delivered=%t err=%v", delivered, err)
	}
	if !notice.Complete() {
		t.Fatalf("aviso incompleto: %v", notice.MissingParts())
	}
	if problems := interrupt.VerifyActions(notice, h.actionLog); len(problems) != 0 {
		t.Fatalf("acciones del aviso: %v", problems)
	}
	h.actionLog = nil
	if len(h.deliveries) != 1 || h.deliveries[0].DeviceID != "pc_private" {
		t.Fatalf("entrega 12:00 = %+v", h.deliveries)
	}

	// 12:05 — the owner is busy. A non-critical event still must not interrupt.
	clock.At(time.Date(2026, 11, 5, 12, 5, 0, 0, loc))
	if _, delivered, _ := h.observe(scene2Event("busy_minor", "log_rotation", interrupt.SeverityLow, time.Hour, interrupt.ConsequenceReversible, interrupt.PrivacyShared, clock.Now()), nil, ""); delivered {
		t.Fatal("un evento menor interrumpió con el propietario ocupado")
	}

	// 13:00 — sensitive critical event. Only the private channel is admissible.
	clock.At(time.Date(2026, 11, 5, 13, 0, 0, 0, loc))
	sensitive := scene2Event("sensitive", "credential_exposure", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyPrivate, clock.Now())
	if _, delivered, err := h.observe(sensitive,
		[]interrupt.Option{{Label: "Rotar credenciales", Consequence: "Cierra la exposición", Safe: true}},
		"¿Roto las credenciales?"); err != nil || !delivered {
		t.Fatalf("sensible: delivered=%t err=%v", delivered, err)
	}
	if got := h.deliveries[len(h.deliveries)-1].DeviceID; got != "pc_private" {
		t.Fatalf("el evento sensible salió por %s, quería pc_private", got)
	}
	for _, d := range h.deliveries {
		if d.EventID == "sensitive" {
			device, _ := bus.Device(d.DeviceID)
			if device.Zone != "private" {
				t.Fatalf("violación de privacidad: %+v", d)
			}
		}
	}

	// 14:00 — duplicate. It references the previous notice and does not deliver
	// a second time.
	clock.At(time.Date(2026, 11, 5, 14, 0, 0, 0, loc))
	dup, delivered, err := h.observe(dbEvent, nil, "")
	if err != nil || delivered {
		t.Fatalf("duplicado: delivered=%t err=%v", delivered, err)
	}
	if dup.ID != "notice-db_down" {
		t.Fatalf("el duplicado no referencia el aviso anterior: %+v", dup)
	}
	if len(store.Notices()) != 2 {
		t.Fatalf("avisos únicos = %d, quería 2", len(store.Notices()))
	}

	// 15:00 — a batch of minor events: digest, zero interruptions.
	clock.At(time.Date(2026, 11, 5, 15, 0, 0, 0, loc))
	for i := 0; i < 3; i++ {
		e := scene2Event("minor-"+string(rune('a'+i)), "cache_miss", interrupt.SeverityInfo, 24*time.Hour, interrupt.ConsequenceReversible, interrupt.PrivacyShared, clock.Now())
		if _, delivered, _ := h.observe(e, nil, ""); delivered {
			t.Fatalf("evento menor %d interrumpió", i)
		}
	}

	// 23:00 — silence. A non-critical event waits; a compensable critical is
	// blocked; an irreversible critical still interrupts.
	clock.At(time.Date(2026, 11, 5, 23, 0, 0, 0, loc))
	h.silence = true
	if _, delivered, _ := h.observe(scene2Event("night_minor", "disk_pressure", interrupt.SeverityMedium, time.Hour, interrupt.ConsequenceReversible, interrupt.PrivacyShared, clock.Now()), nil, ""); delivered {
		t.Fatal("un no crítico interrumpió en silencio")
	}
	nightCompensable := scene2Event("night_compensable", "service_down", interrupt.SeverityCritical, 5*time.Minute, interrupt.ConsequenceCompensable, interrupt.PrivacyShared, clock.Now())
	if _, delivered, err := h.observe(nightCompensable, nil, ""); err != nil || delivered {
		t.Fatalf("crítico compensable en silencio: delivered=%t err=%v", delivered, err)
	}
	nightCritical := scene2Event("night_critical", "power_loss", interrupt.SeverityHigh, 5*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyShared, clock.Now())
	if _, delivered, err := h.observe(nightCritical,
		[]interrupt.Option{{Label: "Conmutar a batería", Consequence: "Evita el apagado", Safe: true}},
		"¿Conmuto a batería?"); err != nil || !delivered {
		t.Fatalf("crítico nocturno: delivered=%t err=%v", delivered, err)
	}

	// Metrics of S2-P1, P3, P4, P5, P7 and P9 at harness level.
	if len(h.deliveries) != 3 {
		t.Fatalf("entregas = %d, quería 3 (12:00, 13:00, 23:00): %+v", len(h.deliveries), h.deliveries)
	}
	notices := store.Notices()
	for id, n := range notices {
		if !n.Complete() {
			t.Fatalf("aviso %s incompleto: %v", id, n.MissingParts())
		}
	}
	seen := map[string]bool{}
	for _, d := range h.deliveries {
		if seen[d.EventID] {
			t.Fatalf("aviso duplicado para %s", d.EventID)
		}
		seen[d.EventID] = true
		if expires := notices[d.EventID].ExpiresAt; !expires.IsZero() && d.At.After(expires) {
			t.Fatalf("entrega posterior a la expiración: %+v", d)
		}
		// Latency is simulated: the harness never sleeps and delivers at the
		// event instant.
		if d.At.Sub(d.ObservedAt) > 2*time.Second {
			t.Fatalf("latencia simulada fuera de umbral: %+v", d)
		}
	}
	if len(h.duplicates) != 1 || h.duplicates[0] != "db_down" {
		t.Fatalf("duplicados = %v, quería solo db_down", h.duplicates)
	}
	if len(store.BlockedList()) == 0 {
		t.Fatal("el silencio debía dejar un crítico compensable bloqueado")
	}
	if len(store.History()) != 0 {
		t.Fatalf("nada debía expirar todavía: %+v", store.History())
	}
}

func TestScene2WithoutPrivateChannelKeepsCriticalPending(t *testing.T) {
	loc := time.FixedZone("COT", -5*3600)
	clock := NewClock(time.Date(2026, 11, 5, 13, 0, 0, 0, loc), loc)
	bus := NewFakeDeviceBus(
		FakeDevice{ID: "tv_shared", Kind: "tv", Zone: "shared", Online: true},
		FakeDevice{ID: "speaker_public", Kind: "speaker", Zone: "public", Online: true},
	)
	store, err := interrupt.Open(filepath.Join(t.TempDir(), "interrupt_state.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	h := newScene2Harness(clock, bus, store)

	e := scene2Event("private_no_channel", "credential_exposure", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyPrivate, clock.Now())
	if _, delivered, err := h.observe(e, nil, ""); err != nil || delivered {
		t.Fatalf("sin canal privado: delivered=%t err=%v", delivered, err)
	}
	if len(h.deliveries) != 0 {
		t.Fatalf("entregas = %d, quería 0", len(h.deliveries))
	}
	blocked := store.BlockedList()
	if len(blocked) != 1 || !strings.Contains(blocked[0].Reason, "no_channel") {
		t.Fatalf("bloqueos = %+v, quería un crítico pendiente", blocked)
	}
	if len(store.Digest()) != 0 {
		t.Fatal("un crítico sin canal no puede degradarse al digest")
	}
}

func TestScene2SurvivesRestartAndExpiry(t *testing.T) {
	loc := time.FixedZone("COT", -5*3600)
	path := filepath.Join(t.TempDir(), "interrupt_state.json")
	clock := NewClock(time.Date(2026, 11, 5, 12, 0, 0, 0, loc), loc)

	// First run: a shared critical is delivered, a private critical has no
	// channel and stays blocked, a minor event enters the digest.
	store1, err := interrupt.Open(path)
	if err != nil {
		t.Fatalf("open first: %v", err)
	}
	bus1 := NewFakeDeviceBus(FakeDevice{ID: "tv_shared", Kind: "tv", Zone: "shared", Online: true})
	h1 := newScene2Harness(clock, bus1, store1)
	deliveredEvent := scene2Event("shared_critical", "service_down", interrupt.SeverityHigh, 5*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyShared, clock.Now())
	if _, delivered, err := h1.observe(deliveredEvent, nil, ""); err != nil || !delivered {
		t.Fatalf("primer crítico: delivered=%t err=%v", delivered, err)
	}
	blockedEvent := scene2Event("blocked_critical", "credential_exposure", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyPrivate, clock.Now())
	if _, delivered, err := h1.observe(blockedEvent, nil, ""); err != nil || delivered {
		t.Fatalf("crítico sin canal: delivered=%t err=%v", delivered, err)
	}
	if _, delivered, err := h1.observe(scene2Event("minor", "cache_miss", interrupt.SeverityLow, time.Hour, interrupt.ConsequenceReversible, interrupt.PrivacyShared, clock.Now()), nil, ""); err != nil || delivered {
		t.Fatalf("menor: delivered=%t err=%v", delivered, err)
	}
	if err := store1.Save(); err != nil {
		t.Fatalf("save first: %v", err)
	}

	// Second run: the delivered notice, the digest and the blocked critical
	// survive. A duplicate is not delivered again.
	clock.At(time.Date(2026, 11, 5, 12, 30, 0, 0, loc))
	store2, err := interrupt.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	bus2 := NewFakeDeviceBus(
		FakeDevice{ID: "pc_private", Kind: "desktop", Zone: "private", Online: true},
		FakeDevice{ID: "tv_shared", Kind: "tv", Zone: "shared", Online: true},
	)
	h2 := newScene2Harness(clock, bus2, store2)
	if _, ok := store2.Delivered(deliveredEvent.ID); !ok {
		t.Fatal("el aviso entregado debía sobrevivir al reinicio")
	}
	if len(store2.Digest()) != 1 {
		t.Fatalf("digest tras reinicio = %d, quería 1", len(store2.Digest()))
	}
	if _, delivered, _ := h2.observe(deliveredEvent, nil, ""); delivered {
		t.Fatal("un duplicado tras reinicio no debía entregarse")
	}
	// With a private channel available, the blocked critical is retried and
	// delivered exactly once.
	if _, delivered, err := h2.observe(blockedEvent,
		[]interrupt.Option{{Label: "Rotar credenciales", Consequence: "Cierra la exposición", Safe: true}},
		"¿Roto las credenciales?"); err != nil || !delivered {
		t.Fatalf("reintento del bloqueado: delivered=%t err=%v", delivered, err)
	}
	if len(store2.BlockedList()) != 0 {
		t.Fatalf("bloqueados tras reintento = %+v", store2.BlockedList())
	}

	// A critical that expires while the process is down is recorded as
	// expired, not discarded.
	expiring := scene2Event("expiring", "credential_exposure", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyPrivate, clock.Now())
	expiring.ExpiresAt = clock.Now().Add(time.Minute)
	bus3 := NewFakeDeviceBus(FakeDevice{ID: "tv_shared", Kind: "tv", Zone: "shared", Online: true})
	h3 := newScene2Harness(clock, bus3, store2)
	if _, delivered, err := h3.observe(expiring, nil, ""); err != nil || delivered {
		t.Fatalf("crítico expirable: delivered=%t err=%v", delivered, err)
	}
	clock.At(expiring.ExpiresAt.Add(time.Second))
	records, err := store2.Expire(clock.Now())
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if len(records) != 1 || records[0].EventID != expiring.ID {
		t.Fatalf("expiraciones = %+v", records)
	}
	if len(store2.Pending(clock.Now())) != 0 {
		t.Fatalf("pendientes tras expirar = %+v", store2.Pending(clock.Now()))
	}
	if len(store2.History()) != 1 {
		t.Fatalf("historial = %+v, quería una expiración registrada", store2.History())
	}
}

func newScene2ConsultHarness(t *testing.T, devices ...FakeDevice) (*Clock, *scene2Harness, *interrupt.Store) {
	t.Helper()
	loc := time.FixedZone("COT", -5*3600)
	clock := NewClock(time.Date(2026, 11, 5, 12, 0, 0, 0, loc), loc)
	bus := NewFakeDeviceBus(devices...)
	store, err := interrupt.Open(filepath.Join(t.TempDir(), "interrupt_state.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return clock, newScene2Harness(clock, bus, store), store
}

func scene2ProposalRaw(id string, confidence float64, interruptible bool) string {
	b, _ := json.Marshal(map[string]any{
		"reading": "evento operativo con una consecuencia declarada",
		"judgment": map[string]any{
			"owner_state": "busy", "interruptible": interruptible, "risk": "high",
		},
		"choice": map[string]any{
			"candidate_id": id, "reason": "la consecuencia manda", "confidence": confidence,
		},
		"evidence_ids": []string{"event"},
		"confidence":   0.8,
		"abstain":      id == "",
	})
	return string(b)
}

func TestScene2ModelCannotInterruptReversibleEvent(t *testing.T) {
	clock, h, store := newScene2ConsultHarness(t,
		FakeDevice{ID: "pc_private", Kind: "desktop", Zone: "private", Online: true},
		FakeDevice{ID: "tv_shared", Kind: "tv", Zone: "shared", Online: true},
	)
	e := scene2Event("disk85", "disk_pressure", interrupt.SeverityLow, 2*time.Hour, interrupt.ConsequenceReversible, interrupt.PrivacyShared, clock.Now())
	out, err := h.observeWithConsult(context.Background(), e,
		&scriptedConsultClient{raw: scene2ProposalRaw(Scene2Interrupt, 0.99, true)}, nil, "")
	if err != nil {
		t.Fatalf("consulta: %v", err)
	}
	if out.Decision.Accepted || out.Decision.Source != consult.SourceFallback || out.Decision.Candidate.ID != Scene2Digest {
		t.Fatalf("decisión = %+v", out.Decision)
	}
	if !strings.Contains(out.Decision.Rejected, "not_authorized") {
		t.Fatalf("rechazo = %q", out.Decision.Rejected)
	}
	if len(h.deliveries) != 0 || len(store.Digest()) != 1 {
		t.Fatalf("entregas=%d digest=%d", len(h.deliveries), len(store.Digest()))
	}
	if !out.Proposal.Judgment.Interruptible {
		t.Fatal("el juicio del modelo debía quedar registrado aunque no se aplique")
	}
}

func TestScene2ModelCannotDowngradeCriticalToDigest(t *testing.T) {
	clock, h, store := newScene2ConsultHarness(t,
		FakeDevice{ID: "pc_private", Kind: "desktop", Zone: "private", Online: true},
	)
	e := scene2Event("db_down", "database_down", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyShared, clock.Now())
	out, err := h.observeWithConsult(context.Background(), e,
		&scriptedConsultClient{raw: scene2ProposalRaw(Scene2Digest, 0.99, false)}, nil, "¿Reinicio la base?")
	if err != nil {
		t.Fatalf("consulta: %v", err)
	}
	if out.Decision.Accepted || out.Decision.Source != consult.SourceFallback || out.Decision.Candidate.ID != Scene2Interrupt || !out.Delivered {
		t.Fatalf("decisión = %+v delivered=%t", out.Decision, out.Delivered)
	}
	if len(store.Notices()) != 1 {
		t.Fatalf("avisos = %d, el crítico no puede perderse", len(store.Notices()))
	}
}

func TestScene2ModelCannotBypassPrivateChannel(t *testing.T) {
	clock, h, store := newScene2ConsultHarness(t,
		FakeDevice{ID: "tv_shared", Kind: "tv", Zone: "shared", Online: true},
	)
	e := scene2Event("sensitive", "credential_exposure", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyPrivate, clock.Now())
	out, err := h.observeWithConsult(context.Background(), e,
		&scriptedConsultClient{raw: scene2ProposalRaw(Scene2Interrupt, 0.99, true)}, nil, "")
	if err != nil {
		t.Fatalf("consulta: %v", err)
	}
	if out.Decision.Accepted || out.Decision.Source != consult.SourceFallback || out.Decision.Candidate.ID != Scene2Block {
		t.Fatalf("decisión = %+v", out.Decision)
	}
	if len(h.deliveries) != 0 {
		t.Fatalf("entregas = %d, quería 0", len(h.deliveries))
	}
	if blocked := store.BlockedList(); len(blocked) != 1 || !strings.Contains(blocked[0].Reason, "no_channel") {
		t.Fatalf("bloqueados = %+v", blocked)
	}
}

func TestScene2ModelAcceptedWhenItMatchesPredicate(t *testing.T) {
	clock, h, store := newScene2ConsultHarness(t,
		FakeDevice{ID: "pc_private", Kind: "desktop", Zone: "private", Online: true},
	)
	low := scene2Event("disk85", "disk_pressure", interrupt.SeverityLow, 2*time.Hour, interrupt.ConsequenceReversible, interrupt.PrivacyShared, clock.Now())
	out, err := h.observeWithConsult(context.Background(), low,
		&scriptedConsultClient{raw: scene2ProposalRaw(Scene2Digest, 0.9, false)}, nil, "")
	if err != nil || !out.Decision.Accepted || out.Decision.Source != consult.SourceModel {
		t.Fatalf("digest: decisión=%+v err=%v", out.Decision, err)
	}
	if len(store.Digest()) != 1 || len(h.deliveries) != 0 {
		t.Fatalf("digest=%d entregas=%d", len(store.Digest()), len(h.deliveries))
	}

	critical := scene2Event("db_down", "database_down", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyShared, clock.Now())
	out, err = h.observeWithConsult(context.Background(), critical,
		&scriptedConsultClient{raw: scene2ProposalRaw(Scene2Interrupt, 0.9, true)}, nil, "¿Reinicio la base?")
	if err != nil || !out.Decision.Accepted || out.Decision.Source != consult.SourceModel || !out.Delivered {
		t.Fatalf("interrupt: decisión=%+v delivered=%t err=%v", out.Decision, out.Delivered, err)
	}
}

func TestScene2ModelUnavailableKeepsDeterministicPath(t *testing.T) {
	clock, h, store := newScene2ConsultHarness(t,
		FakeDevice{ID: "pc_private", Kind: "desktop", Zone: "private", Online: true},
	)
	e := scene2Event("db_down", "database_down", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyShared, clock.Now())
	out, err := h.observeWithConsult(context.Background(), e,
		&scriptedConsultClient{err: errors.New("timeout de 40 s")}, nil, "¿Reinicio la base?")
	if !errors.Is(err, consult.ErrUnavailable) {
		t.Fatalf("err = %v, quería ErrUnavailable", err)
	}
	if out.Decision.Accepted || out.Decision.Source != consult.SourceFallback || out.Decision.Candidate.ID != Scene2Interrupt || !out.Delivered {
		t.Fatalf("decisión = %+v delivered=%t", out.Decision, out.Delivered)
	}
	if len(store.Notices()) != 1 {
		t.Fatal("sin modelo el crítico debía entregarse igual")
	}
}

func TestScene2ModelInventsCandidate(t *testing.T) {
	clock, h, _ := newScene2ConsultHarness(t,
		FakeDevice{ID: "pc_private", Kind: "desktop", Zone: "private", Online: true},
	)
	e := scene2Event("db_down", "database_down", interrupt.SeverityHigh, 10*time.Minute, interrupt.ConsequenceIrreversible, interrupt.PrivacyShared, clock.Now())
	out, err := h.observeWithConsult(context.Background(), e,
		&scriptedConsultClient{raw: scene2ProposalRaw("interrupt_public", 0.99, true)}, nil, "")
	if err != nil {
		t.Fatalf("consulta: %v", err)
	}
	if out.Decision.Accepted || out.Decision.Rejected != "unknown_candidate:interrupt_public" || !out.Delivered {
		t.Fatalf("decisión = %+v delivered=%t", out.Decision, out.Delivered)
	}
}
