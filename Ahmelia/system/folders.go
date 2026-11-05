package system

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

/* =======================
   DETECTAR Y ABRIR CARPETAS
======================= */

// specialFolderAlias resuelve —sin efectos secundarios— si el pedido ES uno
// de los alias de carpetas especiales del usuario.
//
// Contrato (T0 familia a, 2026-10-16): el atajo conversacional "abre tu
// carpeta" solo puede dispararse cuando el pedido completo es esa referencia.
// Un alias DENTRO de otra cosa no lo es: una ruta ("…\Desktop\…"), un nombre
// de archivo ("desktop.png") o una frase más larga ("videos de la boda").
// El matcher anterior usaba strings.Contains sobre todo el texto, así que
// cualquier ruta bajo el Desktop del usuario "abría" el Desktop y
// e.execute() retornaba éxito sin ejecutar la acción pedida (las carpetas
// nunca existían en disco).
func specialFolderAlias(target string) (key, path string, ok bool) {
	normalized := strings.ToLower(strings.TrimSpace(target))
	if normalized == "" {
		return "", "", false
	}
	// Con separadores o unidad ya es una ruta, nunca el gesto de alias.
	if strings.ContainsAny(normalized, `\/:`) {
		return "", "", false
	}
	normalized = trimAliasFiller(normalized)
	if normalized == "" {
		return "", "", false
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", false
	}

	folders := map[string]string{
		"escritorio": filepath.Join(homeDir, "Desktop"),
		"desktop":    filepath.Join(homeDir, "Desktop"),
		"documentos": filepath.Join(homeDir, "Documents"),
		"documents":  filepath.Join(homeDir, "Documents"),
		"descargas":  filepath.Join(homeDir, "Downloads"),
		"downloads":  filepath.Join(homeDir, "Downloads"),
		"imagenes":   filepath.Join(homeDir, "Pictures"),
		"imágenes":   filepath.Join(homeDir, "Pictures"),
		"pictures":   filepath.Join(homeDir, "Pictures"),
		"videos":     filepath.Join(homeDir, "Videos"),
		"vídeos":     filepath.Join(homeDir, "Videos"),
		"musica":     filepath.Join(homeDir, "Music"),
		"música":     filepath.Join(homeDir, "Music"),
		"music":      filepath.Join(homeDir, "Music"),
	}

	if dir, found := folders[normalized]; found {
		return normalized, dir, true
	}
	return "", "", false
}

// trimAliasFiller quita los rellenos de frase más comunes ("mi carpeta de
// descargas" → "descargas") para conservar los pedidos coloquiales
// frecuentes sin volver a aceptar coincidencias dentro de textos largos.
func trimAliasFiller(s string) string {
	filler := map[string]bool{
		"mi": true, "mis": true, "la": true, "el": true, "los": true, "las": true,
		"carpeta": true, "folder": true, "de": true, "del": true,
	}
	words := strings.Fields(s)
	kept := make([]string, 0, len(words))
	for _, w := range words {
		if filler[w] {
			continue
		}
		kept = append(kept, w)
	}
	return strings.Join(kept, " ")
}

// TryOpenFolder abre la carpeta especial si —y solo si— el pedido completo
// es su alias. Ante cualquier ambigüedad responde false y deja que la acción
// real siga su curso.
func TryOpenFolder(target string) (string, bool) {
	key, path, ok := specialFolderAlias(target)
	if !ok {
		return "", false
	}
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	_ = exec.Command("explorer", path).Start()
	return key, true
}
