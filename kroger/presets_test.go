package kroger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPresets_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "presets.json")

	in := Presets{"milk": "0001111041700", "eggs": "0001111060933"}
	if err := SavePresets(path, in); err != nil {
		t.Fatalf("SavePresets: %v", err)
	}

	out, err := LoadPresets(path)
	if err != nil {
		t.Fatalf("LoadPresets: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("len mismatch: got %d, want %d", len(out), len(in))
	}
	for k, v := range in {
		if out[k] != v {
			t.Errorf("key %q: got %q, want %q", k, out[k], v)
		}
	}
}

func TestPresets_LoadMissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")

	got, err := LoadPresets(path)
	if err != nil {
		t.Fatalf("LoadPresets on missing file should not error: %v", err)
	}
	if got == nil {
		t.Fatal("LoadPresets should return non-nil empty map, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestPresets_SaveCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "presets.json")

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("test setup: file should not exist yet, got err %v", err)
	}

	if err := SavePresets(path, Presets{"milk": "u"}); err != nil {
		t.Fatalf("SavePresets: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file should exist after SavePresets: %v", err)
	}
}

// withPresetsPath is a tiny helper for tests that need the package-level
// presetsPath var to point somewhere safe (e.g. t.TempDir()).
func withPresetsPath(path string) (restore func()) {
	old := presetsPath
	presetsPath = path
	return func() { presetsPath = old }
}
