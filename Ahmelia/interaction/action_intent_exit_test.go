package interaction

import "testing"

// Regresión del 13 de octubre de 2026: "...las cosas empezaran a salir a
// flote..." cerró Ahmelia en medio de una conversación porque la palabra de
// acción se detectaba en cualquier parte de la frase. La regla es estructural:
// una palabra de acción solo cuenta cuando abre la orden.
func TestActionWordInsideASentenceIsNotACommand(t *testing.T) {
	conversation := []string{
		"que piensas de lo que hemos construido es dificil saber cuando las cosas empezaran a salir a flote, que sabes sobre ti misma?",
		"quiero salir a caminar un rato despues de terminar esto",
		"cuando vamos a salir de este problema con el sistema",
		"¿puedes salir?",
		"el sistema deberia abrir mas posibilidades",
		"me cansa buscar siempre lo mismo",
		"creo que el volumen va a subir con el tiempo",
		"estoy pensando en cambiar el sistema de enemigos",
		"haz eso y abre el archivo",
	}
	for _, text := range conversation {
		if got := inferAction(text).Name; got != "" {
			t.Errorf("%q es conversación y se detectó como acción %q", text, got)
		}
	}
}

func TestCommandsThatOpenTheUtteranceStillResolve(t *testing.T) {
	cases := map[string]string{
		"abre chrome":                  "abrir_programa",
		"Abre Chrome.":                 "abrir_programa",
		"por favor abre chrome":        "abrir_programa",
		"Amelia, abre chrome":          "abrir_programa",
		"Hola Ahmelia, abre Unity.":    "abrir_programa",
		"puedes abrir chrome":          "abrir_programa",
		"quiero abrir chrome":          "abrir_programa",
		"can you open vlc":             "abrir_programa",
		"y abre chrome":                "abrir_programa",
		"sube el volumen":              "volumen_subir",
		"baja el volumen":              "volumen_bajar",
		"cierra la ventana":            "cerrar_ventana",
		"cambia de ventana":            "cambiar_ventana",
		"salir":                        "salir",
		"Amelia salir":                 "salir",
		"quiero salir":                 "salir",
		"apagar ahmelia":               "salir",
		"adiós":                        "salir",
		"adiós, hasta luego":           "salir",
		"por favor salir del programa": "salir",
		"goodbye":                      "salir",
		"exit":                         "salir",
	}
	for text, want := range cases {
		if got := inferAction(text).Name; got != want {
			t.Errorf("%q => %q, want %q", text, got, want)
		}
	}
}
