package kroger

import "testing"

func TestParsePurchaseQty(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"1 bag", 1},
		{"2 lbs", 2},
		{"3", 3},
		{"  4 packages  ", 4},
		{"", 1},
		{"a few", 1},
		{"0 bags", 1}, // zero/negative falls back to 1; cart needs a positive count
		{"12 oz", 12},
	}
	for _, tc := range cases {
		if got := parsePurchaseQty(tc.in); got != tc.want {
			t.Errorf("parsePurchaseQty(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestNeedsWalkthrough(t *testing.T) {
	cases := []struct {
		ing  Ingredient
		want bool
	}{
		// Empty purchase + not flagged staple = needs setup.
		{Ingredient{Name: "flour"}, true},
		// Explicit staple is enough on its own — purchase can be filled later.
		{Ingredient{Name: "salt", Staple: true}, false},
		// Purchase set = already configured.
		{Ingredient{Name: "flour", Purchase: "1 bag"}, false},
	}
	for _, tc := range cases {
		if got := needsWalkthrough(tc.ing); got != tc.want {
			t.Errorf("needsWalkthrough(%+v) = %v, want %v", tc.ing, got, tc.want)
		}
	}
}
