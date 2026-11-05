package personality

import (
	"fmt"
	"math/rand"
)

// ============================================================================
// MESSAGES.GO - Frases habladas de Ahmelia para eventos concretos del Engine
// ============================================================================
//
// Estas funciones eran llamadas desde core/engine.go pero nunca se habían
// definido en el paquete personality. Cada una devuelve una frase lista para
// hablar/mostrar en un punto específico del flujo (selección de archivos,
// apertura de programas, control de sesión, etc).
//
// randomFrom es el único punto que introduce variedad — antes cada función
// de este archivo devolvía siempre el mismo string fijo (incluso
// identity.Introductions ya tenía 4 variantes escritas y GetIntroduction()
// solo devolvía la primera, siempre). Se usa acá para Greeting() y queda
// disponible para cualquier otra respuesta que quiera dejar de repetirse.
func randomFrom(options []string) string {
	if len(options) == 0 {
		return ""
	}
	return options[rand.Intn(len(options))]
}

// Greeting responde a saludos ("hola", "qué tal", "cómo estás") con
// variedad — antes esto no existía en ningún lugar del código: no había
// ni acción de clasificador, ni respuesta, para un saludo simple.
func Greeting() string {
	return randomFrom([]string{
		"Hola, aquí estoy. ¿Qué necesitas?",
		"Todo bien por acá. ¿En qué te ayudo?",
		"Lista para lo que necesites.",
		"Hola. ¿Qué hacemos?",
		"Aquí ando, funcionando bien. ¿Qué se te ofrece?",
	})
}

// --- Flujo de selección / cancelación ---

func FileActionCancelled() string {
	return "Listo, cancelado."
}

func SelectionNotUnderstood() string {
	return "No entendí cuál elegiste. Dime el número o dime 'cancela'."
}

func InvalidSelection() string {
	return "Ese número no es válido. Elige uno de los que te mostré."
}

func AskFileAction(filename string) string {
	return fmt.Sprintf("Tengo seleccionado %s. ¿Lo abro, lo muevo o lo renombro?", filename)
}

// --- Renombrado ---

func FileRenamed(oldName, newName string) string {
	return fmt.Sprintf("Listo, renombré %s a %s.", oldName, newName)
}

// --- Aprendizaje ---

func DidntUnderstandLearning() string {
	return "No entendí bien qué querías que aprendiera. ¿Puedes decírmelo de otra forma?"
}

func LearningSuccess() string {
	return "Entendido, ya aprendí eso."
}

// --- Sesión ---

func Goodbye() string {
	return "Hasta luego."
}

// --- Archivos ---

func FileMoved(filename string) string {
	return fmt.Sprintf("Moví %s.", filename)
}

func FolderCreated(name string) string {
	return fmt.Sprintf("Listo, creé la carpeta %s.", name)
}

func FileActionOpened(filename string) string {
	return fmt.Sprintf("Abriendo %s.", filename)
}

func Searching(target string) string {
	return fmt.Sprintf("Buscando %s...", target)
}

func FileNotFound(target string) string {
	return fmt.Sprintf("No encontré ningún archivo relacionado con %s.", target)
}

func ManyFilesFound(count int) string {
	return fmt.Sprintf("Encontré %d archivos", count)
}

func FileOption(num int, filename string) string {
	return fmt.Sprintf("Opción %d: %s", num, filename)
}

func MoreOptions() string {
	return "y hay más resultados, dime el número si quieres alguno de los primeros cinco"
}

// --- Carpetas ---

func OpeningFolder(folderName string) string {
	return fmt.Sprintf("Abriendo la carpeta %s.", folderName)
}

// --- Programas / navegación ---

func OpeningEdge() string {
	return "Abriendo Edge."
}

func OpeningURL(url string) string {
	return fmt.Sprintf("Abriendo %s.", url)
}

func ProgramNotFound(target string) string {
	return fmt.Sprintf("No encontré ningún programa llamado %s.", target)
}

func AlreadyOpen(name string) string {
	return fmt.Sprintf("%s ya está abierto.", name)
}

func OpeningProgram(name string) string {
	return fmt.Sprintf("Abriendo %s.", name)
}

func OpeningProgramWithUses(name string, uses int) string {
	return fmt.Sprintf("Abriendo %s, lo has usado %d veces.", name, uses)
}

// --- Medios ---

func VLCNotInstalled() string {
	return "No encontré VLC instalado."
}

func PlayingPlaylist() string {
	return "Reproduciendo la lista completa."
}

func PlayingMedia() string {
	return "Reproduciendo."
}

func PlayingMediaNamed(filename string) string {
	return fmt.Sprintf("Reproduciendo %s.", filename)
}

// --- Genérico ---

func UnknownAction() string {
	return "No supe qué hacer con eso."
}
