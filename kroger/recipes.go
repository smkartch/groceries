package kroger

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

var recipesPath = "recipes.json"

type Recipe struct {
	Name        string       `json:"name"`
	Servings    int          `json:"servings,omitempty"`
	Ingredients []Ingredient `json:"ingredients"`
}

type Ingredient struct {
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity,omitempty"`
	Unit     string  `json:"unit,omitempty"`
	// Purchase is the per-recipe override that says what to actually add to
	// the cart — "1 cup flour" in the recipe, "1 bag" here. Format: "<N> <unit>"
	// where N is an integer count. We only parse the leading integer.
	Purchase string `json:"purchase,omitempty"`
	// Staple flags pantry-common ingredients (salt, oil, spices). They don't
	// auto-add at cook time; they go to the "running low?" prompt instead.
	Staple bool `json:"staple,omitempty"`
}

type recipesFile struct {
	Recipes []Recipe `json:"recipes"`
}

func LoadRecipes(path string) ([]Recipe, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Recipe{}, nil
		}
		return nil, fmt.Errorf("open recipes file: %w", err)
	}
	defer f.Close()

	var rf recipesFile
	if err := json.NewDecoder(f).Decode(&rf); err != nil {
		return nil, fmt.Errorf("decode recipes file: %w", err)
	}
	return rf.Recipes, nil
}

func SaveRecipes(path string, recipes []Recipe) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("open recipes file for write: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(recipesFile{Recipes: recipes})
}

// FindRecipe looks up by name, case-insensitive. Returns a pointer into the
// slice so callers can mutate (e.g. fill in missing fields during walk-through)
// and then persist the slice as a whole.
func FindRecipe(recipes []Recipe, name string) (*Recipe, error) {
	target := strings.ToLower(strings.TrimSpace(name))
	for i := range recipes {
		if strings.ToLower(recipes[i].Name) == target {
			return &recipes[i], nil
		}
	}
	return nil, fmt.Errorf("no recipe named %q (try `groceries recipe list`)", name)
}
