package kroger

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"
)

func TestAddToCart_UsesPUTWithCorrectBody(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotAuth   string
		gotCT     string
		gotBody   map[string]interface{}
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")

		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := &client{
		Config:  &Config{LocationID: "loc-xyz"},
		token:   &oauth2.Token{AccessToken: "tok-abc"},
		presets: Presets{"milk": "0001111041700"},
	}

	withBaseURL(server.URL, func() {
		if err := c.AddToCart(context.Background(), "milk", 2); err != nil {
			t.Fatalf("AddToCart: %v", err)
		}
	})

	if gotMethod != http.MethodPut {
		t.Errorf("method: got %q, want PUT — this is the whole point of the fix", gotMethod)
	}
	if gotPath != "/v1/cart/add" {
		t.Errorf("path: got %q, want /v1/cart/add", gotPath)
	}
	if gotAuth != "Bearer tok-abc" {
		t.Errorf("auth: got %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type: got %q", gotCT)
	}

	items, ok := gotBody["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("body items: got %+v", gotBody)
	}
	item := items[0].(map[string]interface{})
	if item["upc"] != "0001111041700" {
		t.Errorf("body upc: got %v, want preset UPC", item["upc"])
	}
	if item["quantity"].(float64) != 2 {
		t.Errorf("body quantity: got %v, want 2", item["quantity"])
	}
}

func TestAddToCart_PropagatesNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errors":[{"reason":"bad upc"}]}`))
	}))
	defer server.Close()

	c := &client{
		Config:  &Config{LocationID: "loc-xyz"},
		token:   &oauth2.Token{AccessToken: "t"},
		presets: Presets{"milk": "x"},
	}

	withBaseURL(server.URL, func() {
		err := c.AddToCart(context.Background(), "milk", 1)
		if err == nil {
			t.Fatal("expected error on 400, got nil")
		}
	})
}

func TestForget_RemovesAndPersists(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/presets.json"

	if err := SavePresets(path, Presets{"milk": "u1", "eggs": "u2"}); err != nil {
		t.Fatalf("SavePresets: %v", err)
	}

	// Swap presetsPath so Forget writes to our temp file.
	old := withPresetsPath(path)
	defer old()

	c := &client{presets: Presets{"milk": "u1", "eggs": "u2"}}
	if err := c.Forget("milk"); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if _, ok := c.presets["milk"]; ok {
		t.Error("in-memory presets still contain milk")
	}

	got, err := LoadPresets(path)
	if err != nil {
		t.Fatalf("LoadPresets: %v", err)
	}
	if _, ok := got["milk"]; ok {
		t.Error("on-disk presets still contain milk")
	}
	if got["eggs"] != "u2" {
		t.Errorf("eggs preset was lost: got %v", got)
	}
}

func TestPin_PersistsAndOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/presets.json"
	old := withPresetsPath(path)
	defer old()

	c := &client{presets: Presets{}}
	if err := c.Pin("milk", "u-first"); err != nil {
		t.Fatalf("Pin: %v", err)
	}
	if err := c.Pin("milk", "u-second"); err != nil {
		t.Fatalf("Pin overwrite: %v", err)
	}

	got, err := LoadPresets(path)
	if err != nil {
		t.Fatalf("LoadPresets: %v", err)
	}
	if got["milk"] != "u-second" {
		t.Errorf("expected milk=u-second, got %v", got)
	}
}

func TestPin_RejectsEmpty(t *testing.T) {
	c := &client{presets: Presets{}}
	if err := c.Pin("", "u"); err == nil {
		t.Error("expected error on empty name")
	}
	if err := c.Pin("milk", ""); err == nil {
		t.Error("expected error on empty upc")
	}
}

func TestForget_UnknownItem(t *testing.T) {
	c := &client{presets: Presets{}}
	if err := c.Forget("nope"); err == nil {
		t.Fatal("expected error forgetting unknown item")
	}
}
