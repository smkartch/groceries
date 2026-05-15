package kroger

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type Presets map[string]string

var presetsPath = "presets.json"

func LoadPresets(path string) (Presets, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Presets{}, nil
		}
		return nil, fmt.Errorf("failed to open presets file: %w", err)
	}
	defer file.Close()

	var presets Presets
	if err := json.NewDecoder(file).Decode(&presets); err != nil {
		return nil, fmt.Errorf("failed to decode presets file: %w", err)
	}
	if presets == nil {
		presets = Presets{}
	}
	return presets, nil
}

func SavePresets(path string, presets Presets) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to open presets file for write: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(presets)
}
