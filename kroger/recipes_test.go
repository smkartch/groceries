package kroger

import (
	"os"
	"testing"
)

func TestRecipes_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/recipes.json"

	want := []Recipe{
		{
			Name:     "Smash Tacos",
			Servings: 4,
			Ingredients: []Ingredient{
				{Name: "ground beef", Quantity: 1, Unit: "lb", Purchase: "1 lb"},
				{Name: "salt", Staple: true},
			},
		},
	}
	if err := SaveRecipes(path, want); err != nil {
		t.Fatalf("SaveRecipes: %v", err)
	}
	got, err := LoadRecipes(path)
	if err != nil {
		t.Fatalf("LoadRecipes: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Smash Tacos" || got[0].Servings != 4 || len(got[0].Ingredients) != 2 {
		t.Fatalf("round trip lost data: %+v", got)
	}
	if got[0].Ingredients[1].Staple != true {
		t.Errorf("staple flag lost")
	}
}

func TestLoadRecipes_MissingFile(t *testing.T) {
	got, err := LoadRecipes("does-not-exist.json")
	if err != nil {
		t.Fatalf("missing file should be empty list, got error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty list, got %v", got)
	}
}

func TestFindRecipe_CaseInsensitive(t *testing.T) {
	recipes := []Recipe{
		{Name: "Smash Tacos"},
		{Name: "Pad Thai"},
	}
	r, err := FindRecipe(recipes, "smash tacos")
	if err != nil {
		t.Fatalf("FindRecipe: %v", err)
	}
	if r.Name != "Smash Tacos" {
		t.Errorf("got %q", r.Name)
	}
}

func TestFindRecipe_NotFound(t *testing.T) {
	if _, err := FindRecipe([]Recipe{{Name: "x"}}, "y"); err == nil {
		t.Error("expected not-found error")
	}
}

func TestFindRecipe_ReturnsMutablePointer(t *testing.T) {
	recipes := []Recipe{{Name: "Smash Tacos", Ingredients: []Ingredient{{Name: "salt"}}}}
	r, _ := FindRecipe(recipes, "smash tacos")
	r.Ingredients[0].Staple = true
	if !recipes[0].Ingredients[0].Staple {
		t.Error("mutation through FindRecipe pointer did not affect underlying slice")
	}
}

func TestLoadRecipes_RejectsLegacyStringIngredients(t *testing.T) {
	// Old recipes.json used `"ingredients": ["x", "y"]` — strings, not objects.
	// We migrate by rewriting the file; we don't silently accept the old shape.
	dir := t.TempDir()
	path := dir + "/recipes.json"
	if err := os.WriteFile(path, []byte(`{"recipes":[{"name":"x","ingredients":["a","b"]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRecipes(path); err == nil {
		t.Error("expected error decoding legacy string-ingredient shape")
	}
}
