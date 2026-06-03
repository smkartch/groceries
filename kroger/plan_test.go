package kroger

import (
	"os"
	"testing"
)

func TestScalePackages(t *testing.T) {
	cases := []struct {
		name                             string
		base, baseServings, wantServings int
		want                             int
	}{
		// The two examples called out in the plan: 1 bag of flour stays 1 bag
		// from 4→6 servings, and only doubles when servings actually double.
		{"4to6 stays", 1, 4, 6, 1},
		{"4to8 doubles", 1, 4, 8, 2},
		{"4to12 triples", 1, 4, 12, 3},
		{"same servings", 2, 4, 4, 2},
		// Smaller batch can't buy a fraction of a unit — floor at the override.
		{"smaller batch", 1, 4, 2, 1},
		// Base count above 1 multiplies with the whole-multiple factor.
		{"two-pack doubles", 2, 4, 8, 4},
		{"two-pack modest bump stays", 2, 4, 6, 2},
		// Recipe with no servings can't scale — return the override as-is.
		{"no base servings", 3, 0, 8, 3},
		// Defensive: a zero override still buys at least one unit.
		{"zero override floors to one", 0, 4, 8, 2},
	}
	for _, tc := range cases {
		if got := scalePackages(tc.base, tc.baseServings, tc.wantServings); got != tc.want {
			t.Errorf("%s: scalePackages(%d,%d,%d) = %d, want %d",
				tc.name, tc.base, tc.baseServings, tc.wantServings, got, tc.want)
		}
	}
}

func TestPlanPreview_ScalesAggregatesRoutesStaples(t *testing.T) {
	dir := t.TempDir()
	recipesPath = dir + "/recipes.json"
	snoozesPath = dir + "/snoozes.json"
	t.Cleanup(func() {
		recipesPath = "recipes.json"
		snoozesPath = "snoozes.json"
	})

	recipes := []Recipe{
		{
			Name:     "Tacos",
			Servings: 4,
			Ingredients: []Ingredient{
				{Name: "tortillas", Purchase: "1 package"},
				{Name: "salt", Staple: true, Purchase: "1 box"},
			},
		},
		{
			Name:     "Soup",
			Servings: 4,
			Ingredients: []Ingredient{
				{Name: "tortillas", Purchase: "1 package"}, // shared with Tacos → aggregates
				{Name: "garlic powder", Staple: true},      // staple → prompt, not auto-add
			},
		},
	}
	if err := SaveRecipes(recipesPath, recipes); err != nil {
		t.Fatalf("SaveRecipes: %v", err)
	}

	c := &client{presets: Presets{"tortillas": "0007373107159"}}

	prev, err := c.PlanPreview([]PlanEntry{
		{Recipe: "Tacos", Servings: 8}, // 2× → tortillas: 2 packages
		{Recipe: "Soup", Servings: 4},  // 1× → tortillas: 1 package
	})
	if err != nil {
		t.Fatalf("PlanPreview: %v", err)
	}

	// One aggregated non-staple line: tortillas, 2 (from Tacos@8) + 1 (Soup@4) = 3.
	if len(prev.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %+v", len(prev.Lines), prev.Lines)
	}
	line := prev.Lines[0]
	if line.Name != "tortillas" || line.Packages != 3 {
		t.Errorf("tortillas line = %+v, want name=tortillas packages=3", line)
	}
	if !line.HasPreset {
		t.Errorf("tortillas should be flagged as having a preset")
	}

	// Two distinct staples awaiting a decision: salt and garlic powder.
	if len(prev.Staples) != 2 {
		t.Fatalf("expected 2 staple prompts, got %d: %+v", len(prev.Staples), prev.Staples)
	}
}

func TestPlanPreview_SnoozedStapleHidden(t *testing.T) {
	dir := t.TempDir()
	recipesPath = dir + "/recipes.json"
	snoozesPath = dir + "/snoozes.json"
	t.Cleanup(func() {
		recipesPath = "recipes.json"
		snoozesPath = "snoozes.json"
	})

	recipes := []Recipe{{
		Name:        "Stew",
		Servings:    4,
		Ingredients: []Ingredient{{Name: "salt", Staple: true, Purchase: "1 box"}},
	}}
	if err := SaveRecipes(recipesPath, recipes); err != nil {
		t.Fatal(err)
	}
	// Snooze salt far into the future so it never appears as a prompt.
	if err := os.WriteFile(snoozesPath, []byte(`{"salt":"2999-01-01"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &client{presets: Presets{}}
	prev, err := c.PlanPreview([]PlanEntry{{Recipe: "Stew"}})
	if err != nil {
		t.Fatalf("PlanPreview: %v", err)
	}
	if len(prev.Staples) != 0 {
		t.Errorf("snoozed staple should not prompt, got %+v", prev.Staples)
	}
	if len(prev.Snoozed) != 1 || prev.Snoozed[0] != "salt" {
		t.Errorf("expected salt in Snoozed, got %+v", prev.Snoozed)
	}
}
