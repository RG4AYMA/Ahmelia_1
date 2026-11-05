package deliberation

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShadowLogPersistsAndSummarizes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "shadow.jsonl")
	log := NewShadowLog(path)

	first := Record{Outcome: OutcomeAgree, TurnID: "t1", Contract: ContractVersion}
	second := Record{Outcome: OutcomeDisagree, TurnID: "t2", Contract: ContractVersion}
	if err := log.Record(first); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := log.Record(second); err != nil {
		t.Fatalf("Record: %v", err)
	}

	recent := log.Recent(1)
	if len(recent) != 1 || recent[0].TurnID != "t2" {
		t.Fatalf("Recent devolvió %+v", recent)
	}
	counts := log.Counts()
	if counts[OutcomeAgree] != 1 || counts[OutcomeDisagree] != 1 {
		t.Fatalf("conteos inesperados: %+v", counts)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("el archivo de sombra no existe: %v", err)
	}
	defer f.Close()
	lines := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			lines++
		}
	}
	if lines != 2 {
		t.Fatalf("se esperaban 2 líneas JSONL, hay %d", lines)
	}
}

func TestShadowLogWithoutPathStaysInMemory(t *testing.T) {
	log := NewShadowLog("")
	if err := log.Record(Record{Outcome: OutcomeRefine, TurnID: "t1"}); err != nil {
		t.Fatalf("Record en memoria falló: %v", err)
	}
	if len(log.Recent(10)) != 1 {
		t.Fatal("el registro en memoria debe conservar el turno")
	}
	if log.LastError() != nil {
		t.Fatalf("sin ruta no debe haber error de escritura: %v", log.LastError())
	}
}

func TestShadowLogCapacityKeepsNewest(t *testing.T) {
	log := NewShadowLog("")
	log.capacity = 2
	for i := 0; i < 4; i++ {
		_ = log.Record(Record{Outcome: OutcomeAgree, TurnID: string(rune('a' + i))})
	}
	recent := log.Recent(10)
	if len(recent) != 2 || recent[0].TurnID != "c" || recent[1].TurnID != "d" {
		t.Fatalf("el anillo debe conservar los más recientes: %+v", recent)
	}
}
