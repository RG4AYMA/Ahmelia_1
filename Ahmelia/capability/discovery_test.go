package capability

import "testing"

func TestB946DiscoveryFindsCapabilityFromMetadata(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(Capability{ID: "filesystem.create_file", Domain: DomainLocal, Action: "create_file", Description: "crear un archivo en el filesystem", Permission: PermissionAutomatic})
	r.MustRegister(Capability{ID: "unity.create_enemy", Domain: DomainUnity, Action: "create_enemy", Description: "crear un enemigo dentro de Unity", Permission: PermissionAskFirst})

	d := NewDiscovery(r)
	result, ok, err := d.Best("Necesito crear un enemigo en Unity")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("esperaba una capability descubierta")
	}
	if result.Capability.ID != "unity.create_enemy" {
		t.Fatalf("capability incorrecta: %q", result.Capability.ID)
	}
	if result.Score < 2 {
		t.Fatalf("score demasiado bajo: %d", result.Score)
	}
}

func TestB946DiscoveryIsDeterministicOnTie(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(Capability{ID: "b.action", Domain: DomainLocal, Action: "create", Description: "crear", Permission: PermissionAutomatic})
	r.MustRegister(Capability{ID: "a.action", Domain: DomainLocal, Action: "create", Description: "crear", Permission: PermissionAutomatic})

	d := NewDiscovery(r)
	result, ok, err := d.Best("crear")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || result.Capability.ID != "a.action" {
		t.Fatalf("desempate no determinista: %+v", result)
	}
}

func TestB946DiscoveryDoesNotInventCapabilities(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(Capability{ID: "filesystem.create_file", Domain: DomainLocal, Action: "create_file", Description: "crear archivo", Permission: PermissionAutomatic})

	d := NewDiscovery(r)
	results, err := d.Discover("modelar criatura acuática en blender")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("discovery inventó una capability: %+v", results)
	}
}

func TestB946DiscoveryRejectsInvalidQueryAndRegistry(t *testing.T) {
	if _, err := NewDiscovery(NewRegistry()).Discover("   "); err == nil {
		t.Fatal("esperaba error para query vacía")
	}
	if _, err := NewDiscovery(nil).Discover("crear archivo"); err == nil {
		t.Fatal("esperaba error para registry nil")
	}
}
