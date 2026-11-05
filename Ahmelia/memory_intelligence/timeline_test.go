package memory_intelligence

import (
	"ahmelia/interaction"
	"testing"
	"time"

	"ahmelia/memory"
)

func TestParseTimelineQuery_Hoy(t *testing.T) {
	_, _, label, ok := ParseTimelineQuery(interaction.Analyze("que hicimos hoy", interaction.Context{}))
	if !ok || label != "hoy" {
		t.Fatalf("esperaba ok=true label=hoy, obtuve ok=%v label=%q", ok, label)
	}
}

func TestParseTimelineQuery_NoMatch(t *testing.T) {
	_, _, _, ok := ParseTimelineQuery(interaction.Analyze("que hicimos", interaction.Context{}))
	if ok {
		t.Fatalf("esperaba ok=false para una pregunta sin referencia temporal")
	}
}

func TestMostRecentWeekday_HoyMismo(t *testing.T) {
	// Si hoy es lunes y pedimos lunes, debe devolver hoy mismo (diff=0).
	monday := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC) // un lunes real
	got := mostRecentWeekday(monday, time.Monday)
	if !got.Equal(monday) {
		t.Fatalf("esperaba %v, obtuve %v", monday, got)
	}
}

func TestMostRecentWeekday_HaciaAtras(t *testing.T) {
	// Miércoles buscando el lunes más reciente -> 2 días atrás.
	wednesday := time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC)
	want := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	got := mostRecentWeekday(wednesday, time.Monday)
	if !got.Equal(want) {
		t.Fatalf("esperaba %v, obtuve %v", want, got)
	}
}

func TestEpisodesInRange_FiltraYOrdena(t *testing.T) {
	long := &memory.LongMemory{Data: map[string]*memory.Knowledge{}}
	base := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)

	add := func(id string, when time.Time, text string) {
		long.Data[id] = &memory.Knowledge{
			ID: id, Type: memory.KnowContext, Input: text,
			CreatedAt: when, Tags: []string{"episodio"},
		}
	}
	add("episodio_1", base, "primero")
	add("episodio_2", base.Add(2*time.Hour), "segundo")
	add("episodio_3", base.AddDate(0, 0, -5), "fuera de rango")

	from := base.Add(-time.Hour)
	to := base.Add(3 * time.Hour)
	got := EpisodesInRange(long, from, to)

	if len(got) != 2 {
		t.Fatalf("esperaba 2 episodios en rango, obtuve %d", len(got))
	}
	if got[0].Input != "primero" || got[1].Input != "segundo" {
		t.Fatalf("orden cronológico incorrecto: %q, %q", got[0].Input, got[1].Input)
	}
}

func TestDescribeTimeline_SinEpisodios(t *testing.T) {
	desc := DescribeTimeline(nil, "hoy")
	if desc == "" {
		t.Fatal("esperaba una respuesta explícita, no vacía, cuando no hay episodios")
	}
}
