package environment

import "testing"

func TestSyncForegroundAndActiveAppKey(t *testing.T) {
	m := NewManager()

	if _, ok := m.ActiveAppKey(); ok {
		t.Fatal("sin referencia activa todavía")
	}

	m.foregroundProcess = func() (string, uint32, error) { return "code", 4242, nil }
	m.SyncForeground()
	if key, ok := m.ActiveAppKey(); !ok || key != "code" {
		t.Fatalf("ActiveAppKey = %q/%v; se esperaba code/true", key, ok)
	}

	// Las consolas se ignoran (mismo contrato de syncActiveFromOS): el foco
	// en una terminal no debe reescribir la referencia activa.
	m.foregroundProcess = func() (string, uint32, error) { return "powershell", 99, nil }
	m.SyncForeground()
	if key, _ := m.ActiveAppKey(); key != "code" {
		t.Fatalf("la consola no debe pisar la referencia activa: %q", key)
	}
}
