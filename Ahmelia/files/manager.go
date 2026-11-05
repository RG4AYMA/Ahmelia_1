package files

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"ahmelia/shared"
)

/* =======================
   TIPOS
======================= */

type FileResult struct {
	Path         string
	Score        int
	LastModified time.Time
}

/* =======================
   BÚSQUEDA HUMANIZADA (CORREGIDA)
======================= */

func SearchFileHuman(query string) ([]FileResult, error) {
	// "archivo" y "documento" no aportan como keyword de búsqueda:
	// "busca el archivo de matemáticas" solo debe buscar "matematicas".
	keywords := shared.ExtractKeywords(query, "archivo", "documento")
	
	if len(keywords) == 0 {
		return nil, fmt.Errorf("no entendí qué buscar")
	}

	fmt.Printf("🔍 Buscando: %v\n", keywords)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("error obteniendo directorio home: %v", err)
	}

	searchPaths := []string{
		filepath.Join(homeDir, "Desktop"),
		filepath.Join(homeDir, "Escritorio"),
		filepath.Join(homeDir, "Documents"),
		filepath.Join(homeDir, "Documentos"),
		filepath.Join(homeDir, "Downloads"),
		filepath.Join(homeDir, "Descargas"),
		filepath.Join(homeDir, "Pictures"),
		filepath.Join(homeDir, "Imágenes"),
		filepath.Join(homeDir, "Videos"),
		filepath.Join(homeDir, "Music"),
		filepath.Join(homeDir, "Música"),
		filepath.Join(homeDir, "OneDrive"),
	}

	var results []FileResult

	for _, basePath := range searchPaths {
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			continue
		}

		_ = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			score := shared.CalculateSimilarity(keywords, info.Name())

			if score > 0 {
				results = append(results, FileResult{
					Path:         path,
					Score:        score,
					LastModified: info.ModTime(),
				})
			}
			return nil
		})
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no encontré nada con: %v", keywords)
	}

	// Ordenar por score descendente, luego por fecha
	sortFileResults(results)

	return results, nil
}

/* =======================
   ORDENAR RESULTADOS
   Mayor score primero; en empate, el modificado más recientemente.
======================= */

func sortFileResults(results []FileResult) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].LastModified.After(results[j].LastModified)
	})
}

/* =======================
   BÚSQUEDA SIMPLE (LEGACY)
======================= */

func SearchFile(query string) (string, error) {
	results, err := SearchFileHuman(query)
	if err != nil {
		return "", err
	}
	return results[0].Path, nil
}

/* =======================
   MOVER ARCHIVOS
======================= */

func MoveFile(source, destination string) error {
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return fmt.Errorf("archivo origen no existe: %s", source)
	}

	// Si destination es carpeta, conservar nombre del archivo
	if destInfo, err := os.Stat(destination); err == nil && destInfo.IsDir() {
		destination = filepath.Join(destination, filepath.Base(source))
	}

	// No sobreescribir
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("ya existe un archivo en: %s", destination)
	}

	err := os.Rename(source, destination)
	if err != nil {
		// Fallback: copiar + eliminar
		if err = copyFile(source, destination); err != nil {
			return fmt.Errorf("error moviendo archivo: %v", err)
		}
		if err = os.Remove(source); err != nil {
			return fmt.Errorf("archivo copiado pero no se pudo eliminar el original: %v", err)
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, srcInfo.Mode())
}

/* =======================
   RENOMBRAR ARCHIVOS
======================= */

func RenameFile(oldPath, newName string) error {
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return fmt.Errorf("archivo no existe: %s", oldPath)
	}

	newPath := filepath.Join(filepath.Dir(oldPath), newName)

	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("ya existe un archivo con el nombre: %s", newName)
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("error renombrando archivo: %v", err)
	}

	return nil
}

/* =======================
   CREAR CARPETAS
======================= */

func CreateFolder(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("la carpeta ya existe: %s", path)
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("error creando carpeta: %v", err)
	}

	return nil
}

/* =======================
   ABRIR ARCHIVOS
======================= */

func OpenFile(filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("archivo no existe: %s", filePath)
	}

	return exec.Command("cmd", "/C", "start", "", filePath).Start()
}
