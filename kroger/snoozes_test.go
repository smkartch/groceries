package kroger

import (
	"os"
	"testing"
	"time"
)

func TestSnoozes_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/snoozes.json"

	want := Snoozes{
		"garlic powder": time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		"salt":          time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC),
	}
	if err := SaveSnoozes(path, want); err != nil {
		t.Fatalf("SaveSnoozes: %v", err)
	}
	got, err := LoadSnoozes(path)
	if err != nil {
		t.Fatalf("LoadSnoozes: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(got), got)
	}
	if !got["garlic powder"].Equal(want["garlic powder"]) {
		t.Errorf("garlic powder: got %v, want %v", got["garlic powder"], want["garlic powder"])
	}
}

func TestLoadSnoozes_MissingFile(t *testing.T) {
	got, err := LoadSnoozes("does-not-exist.json")
	if err != nil {
		t.Fatalf("missing file should be empty map, got error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestLoadSnoozes_RejectsBadDate(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/snoozes.json"
	if err := os.WriteFile(path, []byte(`{"salt":"not-a-date"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSnoozes(path); err == nil {
		t.Error("expected error on bad date")
	}
}
