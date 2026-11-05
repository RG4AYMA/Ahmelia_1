package core

import "testing"

func TestResolveURLTargetRecognizesWebsiteAlias(t *testing.T) {
	got, ok := resolveURLTarget("youtube")
	if !ok || got != "https://youtube.com" {
		t.Fatalf("got=%q ok=%v, want https://youtube.com", got, ok)
	}
}

func TestResolveURLTargetKeepsExplicitDomain(t *testing.T) {
	got, ok := resolveURLTarget("youtube.com")
	if !ok || got != "https://youtube.com" {
		t.Fatalf("got=%q ok=%v, want https://youtube.com", got, ok)
	}
}
