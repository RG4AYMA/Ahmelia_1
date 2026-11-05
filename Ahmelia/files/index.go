package files

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"
)

const IndexPath = "memory/program_index.json"

type ProgramIndex struct {
	Programs map[string]string `json:"programs"`
	LastScan time.Time         `json:"last_scan"`
	loaded   bool
}

func NewProgramIndex() *ProgramIndex {
	return &ProgramIndex{
		Programs: make(map[string]string),
		loaded:   false,
	}
}

func (p *ProgramIndex) Exists() bool {
	_, err := os.Stat(IndexPath)
	return err == nil
}

func (p *ProgramIndex) Load() {
	if p.loaded {
		return
	}

	data, err := ioutil.ReadFile(IndexPath)
	if err != nil {
		fmt.Println("❌ No se pudo leer program_index.json")
		return
	}

	_ = json.Unmarshal(data, p)
	p.loaded = true

	fmt.Printf("🧠 Índice cargado: %d programas\n", len(p.Programs))
}

func (p *ProgramIndex) Find(name string) string {
	name = strings.ToLower(name)
	p.Load()

	// Coincidencia exacta
	if ruta, ok := p.Programs[name]; ok {
		return ruta
	}

	// Coincidencia parcial
	for prog, ruta := range p.Programs {
		if strings.Contains(prog, name) {
			return ruta
		}
	}

	return ""
}

func (p *ProgramIndex) Save() error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	
	return ioutil.WriteFile(IndexPath, data, 0644)
}
