package kroger

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

var snoozesPath = "snoozes.json"

// snoozeDateLayout is date-only: snoozes are "don't ask until this day",
// not a specific clock time. Stored as YYYY-MM-DD strings in JSON.
const snoozeDateLayout = "2006-01-02"

type Snoozes map[string]time.Time

func LoadSnoozes(path string) (Snoozes, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Snoozes{}, nil
		}
		return nil, fmt.Errorf("open snoozes file: %w", err)
	}
	defer f.Close()

	raw := map[string]string{}
	if err := json.NewDecoder(f).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode snoozes file: %w", err)
	}
	out := Snoozes{}
	for k, v := range raw {
		t, err := time.Parse(snoozeDateLayout, v)
		if err != nil {
			return nil, fmt.Errorf("bad snooze date for %q (%q): %w", k, v, err)
		}
		out[k] = t
	}
	return out, nil
}

func SaveSnoozes(path string, s Snoozes) error {
	raw := map[string]string{}
	for k, v := range s {
		raw[k] = v.Format(snoozeDateLayout)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("open snoozes file for write: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(raw)
}
