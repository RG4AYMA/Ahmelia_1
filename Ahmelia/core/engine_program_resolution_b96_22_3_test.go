package core

import "testing"

func TestFindProgramPrefersDirectShorterName(t *testing.T) {
	idx := ProgramIndex{Programs: map[string]string{
		"unityhubhelper": `C:\Program Files\Unity Hub\resources\UnityHubHelper.exe`,
		"unity hub":      `C:\Program Files\Unity Hub\Unity Hub.exe`,
	}}
	got := findProgram("unity", idx)
	want := `C:\Program Files\Unity Hub\Unity Hub.exe`
	if got != want {
		t.Fatalf("findProgram(unity) = %q, want %q", got, want)
	}
}

func TestFindProgramIsDeterministicForEquivalentCandidates(t *testing.T) {
	idx := ProgramIndex{Programs: map[string]string{
		"alpha tool": `C:\alpha\alpha.exe`,
		"alpha":      `C:\alpha\alpha-direct.exe`,
	}}
	for i := 0; i < 20; i++ {
		if got := findProgram("alpha", idx); got != `C:\alpha\alpha-direct.exe` {
			t.Fatalf("iteration %d: findProgram(alpha) = %q", i, got)
		}
	}
}
