package competence

import "testing"

func real(n, okN int, ctxs ...string) []Attempt {
	var out []Attempt
	for i := 0; i < n; i++ {
		out = append(out, Attempt{Verified: i < okN, ContextKey: ctxs[i%len(ctxs)]})
	}
	return out
}

func TestStandingLadderIsDerivedFromEvidence(t *testing.T) {
	p := DefaultPolicy()
	cases := []struct {
		name string
		e    Evidence
		want Standing
	}{
		{"heard", Evidence{}, StandingHeard},
		{"no oracle caps at unverifiable", Evidence{Compiled: true, Trials: Tally{20, 20}}, StandingUnverifiable},
		{"compiled", Evidence{Compiled: true, HasOracle: true, Trials: Tally{2, 2}}, StandingCompiled},
		{"trialed", Evidence{Compiled: true, HasOracle: true, Trials: Tally{8, 8}}, StandingTrialed},
		{"one context is not demonstrated", Evidence{Compiled: true, HasOracle: true, Trials: Tally{8, 8}, Real: real(8, 8, "A")}, StandingTrialed},
		{"demonstrated", Evidence{Compiled: true, HasOracle: true, Trials: Tally{8, 8}, Real: real(8, 8, "A", "B")}, StandingDemonstrated},
		{"regressed wins", Evidence{Compiled: true, HasOracle: true, Trials: Tally{8, 8}, Real: real(8, 8, "A", "B"), SuiteRegressed: true}, StandingRegressed},
	}
	for _, c := range cases {
		if got, why := Derive(c.e, p); got != c.want {
			t.Fatalf("%s: got %s (%s)", c.name, got, why)
		}
	}
}

func TestLuckyStreakIsNotDemonstration(t *testing.T) {
	e := Evidence{Compiled: true, HasOracle: true, Trials: Tally{8, 8}, Real: real(5, 3, "A", "B")}
	if got, _ := Derive(e, DefaultPolicy()); got != StandingTrialed {
		t.Fatalf("60%% raw rate must not be demonstrated: %s", got)
	}
}
