package personality

import (
	"testing"
	"time"
)

func obs(m *PreferenceModel, k string, o float64, n int) {
	for i := 0; i < n; i++ {
		_ = m.Observe(Experience{Key: k, Outcome: o, Weight: 1, Ref: "w", At: time.Now()})
	}
}

func TestPreferenceNeedsEvidenceAndCanBeRevised(t *testing.T) {
	m := NewPreferenceModel()
	obs(m, "activity:ecosistemas", 1, 2)
	if v := m.View("activity:ecosistemas"); v.Stance != StanceUnformed {
		t.Fatalf("%+v", v)
	}
	obs(m, "activity:ecosistemas", 1, 10)
	if v := m.View("activity:ecosistemas"); v.Stance != StanceLikes {
		t.Fatalf("%+v", v)
	}
	obs(m, "activity:ecosistemas", -1, 40)
	if v := m.View("activity:ecosistemas"); v.Stance != StanceDislikes {
		t.Fatalf("taste not revised by contrary evidence: %+v", v)
	}
}

func TestContradictoryEvidenceIsMixedNotLike(t *testing.T) {
	m := NewPreferenceModel()
	for i := 0; i < 20; i++ {
		o := 1.0
		if i%2 == 0 {
			o = -1
		}
		_ = m.Observe(Experience{Key: "k", Outcome: o, Weight: 1})
	}
	if v := m.View("k"); v.Stance != StanceMixed {
		t.Fatalf("%+v", v)
	}
}

func TestRejectsInvalidExperience(t *testing.T) {
	if NewPreferenceModel().Observe(Experience{Key: "k", Weight: 0}) == nil {
		t.Fatal("accepted")
	}
}
