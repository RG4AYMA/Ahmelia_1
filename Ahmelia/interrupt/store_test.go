package interrupt

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func storeTestNotice(t *testing.T, channel string) Notice {
	t.Helper()
	e := testEvent(SeverityHigh, 10*time.Minute, ConsequenceIrreversible, BlastProject)
	n, err := Compose(ComposeRequest{
		Event:    e,
		NoticeID: "notice-1",
		Options:  []Option{{Label: "Mitigar", Consequence: "Evita la pérdida", Safe: true}},
		Channel:  channel,
	}, e.ObservedAt)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	return n
}

func TestStorePersistsAndQuarantinesCorruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "interrupt_state.json")
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	n := storeTestNotice(t, "pc_private")
	if err := s.RecordDelivery(n, now); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := s.AddDigest(testEvent(SeverityLow, time.Hour, ConsequenceReversible, BlastSelf)); err != nil {
		t.Fatalf("digest: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, ok := reopened.Delivered(n.EventID); !ok {
		t.Fatal("el aviso debía persistir")
	}
	if len(reopened.Digest()) != 1 {
		t.Fatalf("digest = %d, quería 1", len(reopened.Digest()))
	}

	corruptPath := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corruptPath, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}
	corrupt, err := Open(corruptPath)
	if err != nil {
		t.Fatalf("open corrupt: %v", err)
	}
	if len(corrupt.Warnings()) == 0 {
		t.Fatal("un archivo corrupto debía dejar aviso")
	}
	if len(corrupt.Notices()) != 0 {
		t.Fatal("un archivo corrupto no puede inventar avisos")
	}
	if err := corrupt.AddDigest(testEvent(SeverityLow, time.Hour, ConsequenceReversible, BlastSelf)); err != nil {
		t.Fatalf("digest after corrupt: %v", err)
	}
	backups, err := filepath.Glob(corruptPath + ".corrupt-*")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("copias corruptas = %d, quería 1", len(backups))
	}
}

func TestStoreCooldownAndRewriteGuard(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	n := storeTestNotice(t, "pc_private")
	if err := s.RecordDelivery(n, now); err != nil {
		t.Fatalf("record: %v", err)
	}
	if !s.CooldownActive("pc_private", now.Add(10*time.Second), 30*time.Second) {
		t.Fatal("el cooldown debía estar activo a los 10 s")
	}
	if s.CooldownActive("pc_private", now.Add(31*time.Second), 30*time.Second) {
		t.Fatal("el cooldown no debía estar activo a los 31 s")
	}
	if s.CooldownActive("tv_shared", now.Add(10*time.Second), 30*time.Second) {
		t.Fatal("otro canal no hereda el cooldown")
	}

	rewritten := n
	rewritten.WhatHappened = "La base dejó de responder; el sondeo falló."
	if err := s.RecordRewritten(rewritten); err != nil {
		t.Fatalf("reescritura de prosa: %v", err)
	}

	structural := rewritten
	structural.Options = append([]Option(nil), rewritten.Options...)
	structural.Options[0].Label = "Otra opción inventada"
	if err := s.RecordRewritten(structural); err == nil {
		t.Fatal("una reescritura no puede cambiar las opciones")
	}

	decisionChange := rewritten
	decisionChange.DecisionNeeded = "Otra decisión"
	if err := s.RecordRewritten(decisionChange); err == nil {
		t.Fatal("una reescritura no puede cambiar la decisión")
	}
}
