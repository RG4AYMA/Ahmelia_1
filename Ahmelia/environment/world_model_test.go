package environment

import "testing"

// TestWorldSnapshotDescribe prueba la narrativa de World Model v1 con
// los casos del roadmap: "El usuario está desarrollando Ahmelia,
// editor VSCode, archivo engine.go, cambios pendientes: sí".
func TestWorldSnapshotDescribe(t *testing.T) {
	cases := []struct {
		name string
		snap *WorldSnapshot
		want string
	}{
		{
			name: "sin app activa",
			snap: &WorldSnapshot{HasActiveApp: false},
			want: "No tengo suficiente información sobre lo que estás haciendo ahora mismo",
		},
		{
			name: "VSCode con proyecto y archivo, sin cambios",
			snap: &WorldSnapshot{
				HasActiveApp: true,
				AppName:      "Visual Studio Code",
				Activity:     ActivityCoding,
				Project:      "Ahmelia",
				File:         "engine.go",
			},
			want: "Estás programando, en el proyecto Ahmelia, editando engine.go",
		},
		{
			name: "VSCode con cambios sin guardar",
			snap: &WorldSnapshot{
				HasActiveApp:      true,
				AppName:           "Visual Studio Code",
				Activity:          ActivityCoding,
				Project:           "Ahmelia",
				File:              "engine.go",
				HasUnsavedChanges: true,
			},
			want: "Estás programando, en el proyecto Ahmelia, editando engine.go (tienes cambios sin guardar)",
		},
		{
			name: "app sin proyecto/archivo reconocible",
			snap: &WorldSnapshot{
				HasActiveApp: true,
				AppName:      "Steam",
				Activity:     ActivityGaming,
			},
			want: "Estás jugando con Steam",
		},
	}

	for _, c := range cases {
		got := c.snap.Describe()
		if got != c.want {
			t.Errorf("%s:\n  got:  %q\n  want: %q", c.name, got, c.want)
		}
	}
}

// TestActivityByCategoryComplete verifica que todas las categorías
// conocidas (friendly.go) tengan una actividad mapeada, salvo
// CategoryOther y CategoryLauncher, que deliberadamente caen en
// ActivityUnknown (no hay una actividad clara para "otro" o para un
// launcher que no es, en sí, la actividad).
func TestActivityByCategoryComplete(t *testing.T) {
	known := []AppCategory{CategoryDev, CategoryBrowser, CategoryGame, CategoryMedia, CategoryOffice, CategorySystem}
	for _, cat := range known {
		if _, ok := activityByCategory[cat]; !ok {
			t.Errorf("categoría %q sin actividad mapeada", cat)
		}
	}
}
