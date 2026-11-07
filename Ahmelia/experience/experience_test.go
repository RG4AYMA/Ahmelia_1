package experience

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendRecordsAndLessons(t *testing.T) {
	path := filepath.Join(t.TempDir(), "experience.jsonl")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	for _, record := range []Record{
		{At: now, Domain: "programming", ProjectID: "p", Attempt: 1, Stage: "synthesis", Error: "missing test", Outcome: "retry", Lesson: "Go tests go in _test.go"},
		{At: now.Add(time.Second), Domain: "programming", ProjectID: "p", Attempt: 2, Stage: "synthesis", Outcome: "success"},
		{At: now.Add(2 * time.Second), Domain: "other", ProjectID: "q", Attempt: 1, Stage: "synthesis", Outcome: "failure", Lesson: "otra lección"},
	} {
		if err := s.Append(record); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	records, err := s.Records("programming")
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("registros = %d, quería 2", len(records))
	}
	lessons := s.Lessons("programming", 5)
	if len(lessons) != 1 || lessons[0] != "Go tests go in _test.go" {
		t.Fatalf("lecciones = %v", lessons)
	}
}

func TestMissingFileAndCorruptLinesAreSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "experience.jsonl")
	s, _ := Open(path)
	if records, err := s.Records("programming"); err != nil || len(records) != 0 {
		t.Fatalf("archivo ausente: records=%v err=%v", records, err)
	}
	if err := os.WriteFile(path, []byte("{bad json\n"+`{"domain":"programming","attempt":1,"stage":"synthesis","outcome":"success"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	records, err := s.Records("programming")
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	if len(records) != 1 || records[0].Outcome != "success" {
		t.Fatalf("registros = %+v", records)
	}
}

func TestEmptyStoreIsInert(t *testing.T) {
	var s *Store
	if err := s.Append(Record{Domain: "programming"}); err != nil {
		t.Fatalf("nil append: %v", err)
	}
	empty, _ := Open("")
	if err := empty.Append(Record{Domain: "programming", Attempt: 1, Stage: "synthesis", Outcome: "success"}); err != nil {
		t.Fatalf("empty append: %v", err)
	}
	if records, _ := empty.Records("programming"); len(records) != 0 {
		t.Fatalf("store vacío devolvió %+v", records)
	}
}
