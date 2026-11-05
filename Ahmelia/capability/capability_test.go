package capability

import "testing"

func TestPermissionLevel_IsValid(t *testing.T) {
	valid := []PermissionLevel{PermissionAutomatic, PermissionNotify, PermissionAskFirst}
	for _, p := range valid {
		if !p.IsValid() {
			t.Errorf("IsValid(%q) = false, esperaba true", p)
		}
	}

	invalid := []PermissionLevel{"", "quizas", "AUTOMATIC"}
	for _, p := range invalid {
		if p.IsValid() {
			t.Errorf("IsValid(%q) = true, esperaba false", p)
		}
	}
}

func TestCapability_Validate_CompletaEsValida(t *testing.T) {
	c := Capability{
		ID:          "crear_carpeta",
		Domain:      DomainLocal,
		Description: "Crea una carpeta en el sistema de archivos",
		Inputs: []Parameter{
			{Name: "ruta", Description: "ruta/nombre de la carpeta a crear", Required: true},
		},
		Permission:    PermissionAutomatic,
		Preconditions: []string{"la ruta padre debe existir"},
		Action:        "crear_carpeta",
		FailureModes: []FailureMode{
			{Code: "ruta_invalida", Description: "la ruta contiene caracteres no permitidos", Recoverable: true},
		},
	}

	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() = %v, esperaba nil", err)
	}
}

func TestCapability_Validate_SinInputsNiFailureModesSigueSiendoValida(t *testing.T) {
	// Ej: "detener" (actions/unity/stop.go) no necesita parámetros ni
	// tiene modos de falla documentados todavía — eso no la invalida.
	c := Capability{
		ID:         "unity_stop",
		Domain:     DomainUnity,
		Permission: PermissionAutomatic,
		Action:     "detener",
	}

	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() = %v, esperaba nil para una capability sin Inputs/FailureModes", err)
	}
}

func TestCapability_Validate_CamposFaltantes(t *testing.T) {
	cases := []struct {
		name string
		c    Capability
	}{
		{"sin ID", Capability{Domain: DomainLocal, Action: "crear_carpeta", Permission: PermissionAutomatic}},
		{"sin Domain", Capability{ID: "x", Action: "crear_carpeta", Permission: PermissionAutomatic}},
		{"sin Action", Capability{ID: "x", Domain: DomainLocal, Permission: PermissionAutomatic}},
		{"sin Permission", Capability{ID: "x", Domain: DomainLocal, Action: "crear_carpeta"}},
		{"Permission inválido", Capability{ID: "x", Domain: DomainLocal, Action: "crear_carpeta", Permission: "urgente"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.c.Validate(); err == nil {
				t.Fatalf("Validate() = nil, esperaba error para caso %q", tc.name)
			}
		})
	}
}

func TestCapability_AskFirstEsDistintoDeAutomatic(t *testing.T) {
	// Prueba de vocabulario, no de lógica: PermissionAskFirst y
	// PermissionAutomatic deben ser valores distintos y ambos válidos —
	// es la base de datos que B94.3 necesita para decidir si pedir
	// autorización antes de ejecutar.
	if PermissionAskFirst == PermissionAutomatic {
		t.Fatal("PermissionAskFirst y PermissionAutomatic no deberían ser el mismo valor")
	}
	if !PermissionAskFirst.IsValid() || !PermissionAutomatic.IsValid() {
		t.Fatal("ambos niveles deberían ser válidos")
	}
}
