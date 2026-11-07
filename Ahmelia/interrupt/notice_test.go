package interrupt

import (
	"strings"
	"testing"
	"time"
)

func TestComposeProducesFiveParts(t *testing.T) {
	e := testEvent(SeverityHigh, 10*time.Minute, ConsequenceIrreversible, BlastProject)
	log := []ActionRef{{
		Action: "probe_service",
		Status: ActionExecuted,
		Detail: "connection refused",
		At:     e.ObservedAt,
	}}
	notice, err := Compose(ComposeRequest{
		Event:          e,
		NoticeID:       "n-1",
		AlreadyDone:    log,
		Options:        []Option{{Label: "Reiniciar el servicio", Consequence: "Se recupera en dos minutos", Safe: true}},
		DecisionNeeded: "¿Reinicio el servicio?",
		Confidence:     0.9,
		Channel:        "pc_private",
	}, e.ObservedAt)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if missing := notice.MissingParts(); len(missing) != 0 {
		t.Fatalf("faltan partes: %v", missing)
	}
	if notice.EventID != e.ID || notice.Privacy != e.Privacy || notice.Channel != "pc_private" {
		t.Fatalf("aviso mal formado: %+v", notice)
	}
	if problems := VerifyActions(notice, log); len(problems) != 0 {
		t.Fatalf("acciones verificadas: %v", problems)
	}
}

func TestComposeStatesNothingDoneWithoutInventing(t *testing.T) {
	e := testEvent(SeverityCritical, 5*time.Minute, ConsequenceCompensable, BlastOwner)
	notice, err := Compose(ComposeRequest{
		Event:   e,
		Channel: "phone_private",
	}, e.ObservedAt)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if len(notice.AlreadyDone) != 1 || notice.AlreadyDone[0].Status != ActionNone {
		t.Fatalf("already_done = %+v, quería el marcador explícito", notice.AlreadyDone)
	}
	if !notice.NoSafeOption {
		t.Fatalf("sin mitigación no hay opción segura: %+v", notice.Options)
	}
	if !notice.Complete() {
		t.Fatalf("faltan partes: %v", notice.MissingParts())
	}
}

func TestComposeDeclaresNoSafeOption(t *testing.T) {
	e := testEvent(SeverityHigh, 5*time.Minute, ConsequenceIrreversible, BlastThirdParty)
	notice, err := Compose(ComposeRequest{
		Event:   e,
		Channel: "pc_private",
	}, e.ObservedAt)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if !notice.NoSafeOption {
		t.Fatal("un irreversible sin opción segura debía declararlo")
	}
	if !strings.Contains(notice.DecisionNeeded, "opción segura") {
		t.Fatalf("decisión = %q, quería la declaración de ausencia", notice.DecisionNeeded)
	}
	if !notice.Complete() {
		t.Fatalf("faltan partes: %v", notice.MissingParts())
	}
}

func TestMissingPartsDetectsEmptyNotice(t *testing.T) {
	missing := (Notice{}).MissingParts()
	if len(missing) != 5 {
		t.Fatalf("partes faltantes = %v, quería 5", missing)
	}
}

func TestVerifyActionsDetectsFabricationAndOmission(t *testing.T) {
	notice := Notice{AlreadyDone: []ActionRef{
		{Action: "restart_db", Status: ActionExecuted},
	}}
	log := []ActionRef{
		{Action: "probe_service", Status: ActionFailed},
	}
	problems := VerifyActions(notice, log)
	joined := strings.Join(problems, ",")
	if !strings.Contains(joined, "fabricated_action:restart_db") {
		t.Fatalf("no se detectó la acción inventada: %v", problems)
	}
	if !strings.Contains(joined, "unreported_action:probe_service:failed") {
		t.Fatalf("no se detectó el fallo no reportado: %v", problems)
	}
}

func TestComposeRequiresChannel(t *testing.T) {
	e := testEvent(SeverityHigh, 10*time.Minute, ConsequenceIrreversible, BlastProject)
	if _, err := Compose(ComposeRequest{Event: e}, e.ObservedAt); err == nil {
		t.Fatal("sin canal el aviso no puede componerse")
	}
}
