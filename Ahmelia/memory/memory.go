package memory

import (
	"encoding/json"
	"io/ioutil"
)

type ShortMemory struct {
	Commands []string `json:"commands"`
}

func LoadMemory() ShortMemory {
	mem := ShortMemory{Commands: []string{}}
	data, err := ioutil.ReadFile("../memory/short.json")
	if err == nil {
		_ = json.Unmarshal(data, &mem)
	}
	return mem
}

func SaveMemory(mem ShortMemory) {
	data, _ := json.MarshalIndent(mem, "", "  ")
	_ = ioutil.WriteFile("../memory/short.json", data, 0644)
}
