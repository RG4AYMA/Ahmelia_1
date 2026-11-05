package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WriteFileAtomic escribe tmp + fsync + rename y conserva la versión previa
// como <path>.bak. Un corte de energía nunca deja un archivo a medias.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if _, statErr := os.Stat(path); statErr == nil {
		_ = os.Rename(path, path+".bak")
	}
	return os.Rename(tmp, path)
}

// QuarantineCorrupt aparta un archivo ilegible (no lo sobrescribe en silencio).
func QuarantineCorrupt(path string) (string, error) {
	dst := fmt.Sprintf("%s.corrupt-%d", path, time.Now().Unix())
	return dst, os.Rename(path, dst)
}
