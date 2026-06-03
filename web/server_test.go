package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"groceries/kroger"
)

// fakeClient implements kroger.KrogerClient with just enough behaviour to
// exercise the HTTP layer without touching Kroger or the filesystem.
type fakeClient struct {
	recipes   []kroger.Recipe
	preview   *kroger.Preview
	addResult *kroger.AddResult
	lastAdd   kroger.AddPlanRequest
}

func (f *fakeClient) Init(context.Context, string) error                        { return nil }
func (f *fakeClient) AddToCart(context.Context, string, int) error              { return nil }
func (f *fakeClient) AddManyToCart(context.Context, []kroger.CartRequest) error { return nil }
func (f *fakeClient) Cook(context.Context, string) error                        { return nil }
func (f *fakeClient) RecipeList() ([]kroger.Recipe, error)                      { return f.recipes, nil }
func (f *fakeClient) RecipeAdd(string) error                                    { return nil }
func (f *fakeClient) RecipeEdit(string) error                                   { return nil }
func (f *fakeClient) Search(context.Context, string, int) ([]kroger.Product, error) {
	return nil, nil
}
func (f *fakeClient) ListPresets() kroger.Presets { return kroger.Presets{} }
func (f *fakeClient) Pin(string, string) error    { return nil }
func (f *fakeClient) Forget(string) error         { return nil }
func (f *fakeClient) PlanPreview([]kroger.PlanEntry) (*kroger.Preview, error) {
	return f.preview, nil
}
func (f *fakeClient) AddPlan(_ context.Context, req kroger.AddPlanRequest) (*kroger.AddResult, error) {
	f.lastAdd = req
	return f.addResult, nil
}

func newTestServer(t *testing.T, fc *fakeClient) http.Handler {
	t.Helper()
	s, err := NewServer(fc)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return s.mux
}

func TestServesIndexAndAssets(t *testing.T) {
	h := newTestServer(t, &fakeClient{})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET / = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Kitchen Garden") {
		t.Errorf("index.html not served; body=%.80q", rr.Body.String())
	}
}

func TestRecipesEndpoint(t *testing.T) {
	fc := &fakeClient{recipes: []kroger.Recipe{{Name: "Tacos", Servings: 4}}}
	h := newTestServer(t, fc)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/recipes", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/recipes = %d", rr.Code)
	}
	var got []kroger.Recipe
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Tacos" {
		t.Errorf("got %+v", got)
	}
}

func TestPreviewEndpoint(t *testing.T) {
	fc := &fakeClient{preview: &kroger.Preview{
		Lines: []kroger.PlanLine{{Name: "tortillas", Packages: 2}},
	}}
	h := newTestServer(t, fc)
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"entries":[{"recipe":"Tacos","servings":8}]}`)
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/api/plan/preview", body))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/plan/preview = %d (%s)", rr.Code, rr.Body.String())
	}
	var prev kroger.Preview
	if err := json.Unmarshal(rr.Body.Bytes(), &prev); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(prev.Lines) != 1 || prev.Lines[0].Packages != 2 {
		t.Errorf("got %+v", prev)
	}
}

func TestPreviewRejectsEmptyPlan(t *testing.T) {
	h := newTestServer(t, &fakeClient{})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/api/plan/preview", strings.NewReader(`{"entries":[]}`)))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("empty plan should be 400, got %d", rr.Code)
	}
}

func TestCartEndpointForwardsRequest(t *testing.T) {
	fc := &fakeClient{addResult: &kroger.AddResult{
		Added: []kroger.AddedLine{{Name: "tortillas", UPC: "0007373107159", Quantity: 2}},
	}}
	h := newTestServer(t, fc)
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"lines":[{"Name":"tortillas","Quantity":2}],"snoozes":[{"name":"salt","until":"2026-12-01"}]}`)
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/api/cart", body))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/cart = %d (%s)", rr.Code, rr.Body.String())
	}
	if len(fc.lastAdd.Lines) != 1 || fc.lastAdd.Lines[0].Name != "tortillas" || fc.lastAdd.Lines[0].Quantity != 2 {
		t.Errorf("cart request not forwarded faithfully: %+v", fc.lastAdd)
	}
	if len(fc.lastAdd.Snoozes) != 1 || fc.lastAdd.Snoozes[0].Name != "salt" {
		t.Errorf("snoozes not forwarded: %+v", fc.lastAdd.Snoozes)
	}
	var res kroger.AddResult
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(res.Added) != 1 {
		t.Errorf("got %+v", res)
	}
}
