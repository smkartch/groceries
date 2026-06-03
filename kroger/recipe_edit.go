package kroger

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RecipeList returns every recipe in recipes.json. The CLI uses this for
// `groceries recipe list`.
func (this *client) RecipeList() ([]Recipe, error) {
	return LoadRecipes(recipesPath)
}

// RecipeAdd appends a new empty-ish recipe stub with the given name and
// drops the user straight into $EDITOR to fill it out.
func (this *client) RecipeAdd(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("recipe name cannot be empty")
	}
	recipes, err := LoadRecipes(recipesPath)
	if err != nil {
		return err
	}
	if _, err := FindRecipe(recipes, name); err == nil {
		return fmt.Errorf("recipe %q already exists — use `recipe edit` instead", name)
	}
	recipes = append(recipes, Recipe{
		Name:        name,
		Servings:    1,
		Ingredients: []Ingredient{{Name: "rename me"}},
	})
	if err := SaveRecipes(recipesPath, recipes); err != nil {
		return err
	}
	fmt.Printf("✨ Created stub for %q. Opening editor…\n", name)
	return this.RecipeEdit(name)
}

// RecipeEdit pulls one recipe out of recipes.json, writes it to a temp file,
// hands the temp file to $EDITOR, and on close writes it back. We edit a
// single recipe (not the whole file) so a botched edit only ruins one entry.
func (this *client) RecipeEdit(name string) error {
	recipes, err := LoadRecipes(recipesPath)
	if err != nil {
		return err
	}
	r, err := FindRecipe(recipes, name)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp("", "groceries-recipe-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		tmp.Close()
		return fmt.Errorf("write recipe to temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}

	editor := pickEditor()
	cmd := exec.Command(editor, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor %q exited with error: %w", editor, err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("re-read temp: %w", err)
	}
	var updated Recipe
	if err := json.Unmarshal(data, &updated); err != nil {
		// Don't silently keep the old version — surface the JSON error so the
		// user can fix it (we still have the original on disk, untouched).
		return fmt.Errorf("recipe JSON is invalid (recipes.json unchanged): %w", err)
	}
	if strings.TrimSpace(updated.Name) == "" {
		return fmt.Errorf("recipe must have a non-empty name (recipes.json unchanged)")
	}
	*r = updated

	if err := SaveRecipes(recipesPath, recipes); err != nil {
		return err
	}
	fmt.Printf("✅ Saved %q.\n", updated.Name)
	return nil
}

// pickEditor honors $VISUAL, then $EDITOR, then falls back to nano.
func pickEditor() string {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v
		}
	}
	return "nano"
}
