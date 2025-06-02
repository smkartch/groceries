package kroger

import (
	"encoding/json"
	"fmt"
	"os"
)

type Presets map[string]string

func LoadPresets(path string) (Presets, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open presets file: %w", err)
	}
	defer file.Close()

	var presets Presets
	if err := json.NewDecoder(file).Decode(&presets); err != nil {
		return nil, fmt.Errorf("failed to decode presets file: %w", err)
	}

	return presets, nil
}
