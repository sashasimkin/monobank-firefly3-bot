package config

import (
	"encoding/json"
	"io"
	"os"
)

func Read(path string) (Config, error) {
	var config Config

	// open file
	file, err := os.Open(path)
	if err != nil {
		return config, err
	}
	defer file.Close()

	// read file
	bytes, err := io.ReadAll(file)
	if err != nil {
		return config, err
	}

	// read file ot config struct
	if err := json.Unmarshal(bytes, &config); err != nil {
		return config, err
	}

	return config, nil
}
