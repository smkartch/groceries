package kroger

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchProducts_RequestShape(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotTerm, gotLocation, gotLimit string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotTerm = r.URL.Query().Get("filter.term")
		gotLocation = r.URL.Query().Get("filter.locationId")
		gotLimit = r.URL.Query().Get("filter.limit")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[
			{"productId":"p1","upc":"0001111041700","brand":"Kroger","description":"2% Reduced Fat Milk","items":[{"itemId":"i1","size":"1 gal"}]},
			{"productId":"p2","upc":"0001111041800","brand":"Simple Truth","description":"Organic Whole Milk","items":[{"itemId":"i2","size":"64 fl oz"}]}
		]}`))
	}))
	defer server.Close()

	withBaseURL(server.URL, func() {
		got, err := SearchProducts(context.Background(), "tok-abc", "loc-123", "milk", 5)
		if err != nil {
			t.Fatalf("SearchProducts: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d products, want 2", len(got))
		}
		if got[0].UPC != "0001111041700" {
			t.Errorf("UPC: got %q", got[0].UPC)
		}
		if got[0].Brand != "Kroger" || got[0].Description != "2% Reduced Fat Milk" {
			t.Errorf("brand/desc: got %+v", got[0])
		}
		if len(got[0].Items) == 0 || got[0].Items[0].Size != "1 gal" {
			t.Errorf("size: got %+v", got[0].Items)
		}
	})

	if gotMethod != "GET" {
		t.Errorf("method: got %q, want GET", gotMethod)
	}
	if gotPath != "/v1/products" {
		t.Errorf("path: got %q, want /v1/products", gotPath)
	}
	if gotAuth != "Bearer tok-abc" {
		t.Errorf("auth: got %q, want %q", gotAuth, "Bearer tok-abc")
	}
	if gotTerm != "milk" {
		t.Errorf("filter.term: got %q, want %q", gotTerm, "milk")
	}
	if gotLocation != "loc-123" {
		t.Errorf("filter.locationId: got %q, want %q", gotLocation, "loc-123")
	}
	if gotLimit != "5" {
		t.Errorf("filter.limit: got %q, want %q", gotLimit, "5")
	}
}

func TestSearchProducts_DefaultsLimit(t *testing.T) {
	var gotLimit string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLimit = r.URL.Query().Get("filter.limit")
		w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	withBaseURL(server.URL, func() {
		_, err := SearchProducts(context.Background(), "t", "loc", "x", 0)
		if err != nil {
			t.Fatalf("SearchProducts: %v", err)
		}
	})

	if gotLimit != "10" {
		t.Errorf("default limit: got %q, want 10", gotLimit)
	}
}

func TestSearchProducts_PropagatesHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	withBaseURL(server.URL, func() {
		_, err := SearchProducts(context.Background(), "t", "loc", "x", 5)
		if err == nil {
			t.Fatal("expected error on 401, got nil")
		}
	})
}

func TestProductDisplay_PriceFormatting(t *testing.T) {
	mk := func(reg, promo float64) Product {
		p := Product{Brand: "Acme", Description: "Widget", UPC: "U1"}
		item := ProductItem{Size: "1 ea"}
		item.Price.Regular = reg
		item.Price.Promo = promo
		p.Items = []ProductItem{item}
		return p
	}

	cases := []struct {
		name, want string
		prod       Product
	}{
		{"no price", "Acme Widget · 1 ea [U1]", mk(0, 0)},
		{"regular only", "Acme Widget · 1 ea · $3.49 [U1]", mk(3.49, 0)},
		{"promo cheaper than regular", "Acme Widget · 1 ea · $2.99 (was $3.49) [U1]", mk(3.49, 2.99)},
		{"promo same as regular ignored", "Acme Widget · 1 ea · $3.49 [U1]", mk(3.49, 3.49)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.prod.Display(); got != tc.want {
				t.Errorf("\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

// withBaseURL temporarily swaps the package's apiBaseURL for a test server.
func withBaseURL(url string, fn func()) {
	old := apiBaseURL
	apiBaseURL = url
	defer func() { apiBaseURL = old }()
	fn()
}
