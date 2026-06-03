package kroger

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// plan.go is the headless counterpart to cook.go. Where Cook drives the
// meal-planning flow through stdin/stdout prompts, the functions here take the
// same decisions as plain data so the web UI (web/server.go) can drive them
// over HTTP. The Kroger-facing primitives — presets, Search, putCart — are
// shared with the CLI; only the I/O layer differs.

// PlanEntry is one recipe the cook wants to make, at a chosen serving count.
// Servings of 0 means "use the recipe's own default".
type PlanEntry struct {
	Recipe   string `json:"recipe"`
	Servings int    `json:"servings"`
}

// PlanLine is one non-staple ingredient destined for the cart, after scaling
// and aggregation across every recipe in the plan.
type PlanLine struct {
	Name      string `json:"name"`
	Packages  int    `json:"packages"`
	ScaledQty string `json:"scaledQty,omitempty"` // human hint, e.g. "1.5 cups" — informational
	HasPreset bool   `json:"hasPreset"`
	From      string `json:"from"` // recipe(s) this line came from, for display
}

// StaplePrompt is a staple ingredient that needs a "running low?" decision
// before it can join the cart. Snoozed staples never appear here.
type StaplePrompt struct {
	Name     string `json:"name"`
	Packages int    `json:"packages"`
	From     string `json:"from"`
}

// Preview is what the planner shows before anything is added: the auto-add
// lines, the staples awaiting a decision, and which staples were skipped
// because they're still snoozed (shown so the cook isn't surprised).
type Preview struct {
	Lines   []PlanLine     `json:"lines"`
	Staples []StaplePrompt `json:"staples"`
	Snoozed []string       `json:"snoozed"`
}

// StapleSnooze records a "no, and don't ask again until <date>" decision.
type StapleSnooze struct {
	Name  string `json:"name"`
	Until string `json:"until"` // YYYY-MM-DD
}

// AddPlanRequest is everything the planner sends when the cook hits "add to
// cart": the lines to buy (non-staples plus any staples answered "yes", with
// counts the cook may have nudged), the snooze decisions to persist, and any
// UPC choices made in a prior disambiguation round.
type AddPlanRequest struct {
	Lines   []CartRequest     `json:"lines"`
	Snoozes []StapleSnooze    `json:"snoozes"`
	Chosen  map[string]string `json:"chosen"` // ingredient name -> chosen UPC
}

// Candidate is one product offered to the cook when an ingredient can't be
// resolved automatically.
type Candidate struct {
	UPC     string `json:"upc"`
	Display string `json:"display"`
}

// Unresolved is an ingredient that needs the cook to pick a product before the
// cart can go through.
type Unresolved struct {
	Name       string      `json:"name"`
	Candidates []Candidate `json:"candidates"`
	Note       string      `json:"note,omitempty"` // e.g. "no matches found"
}

// AddedLine is one line that made it into the cart.
type AddedLine struct {
	Name     string `json:"name"`
	UPC      string `json:"upc"`
	Quantity int    `json:"quantity"`
}

// AddResult is the outcome of AddPlan. If Unresolved is non-empty, NOTHING was
// added — the cook resolves the listed items and resubmits with their UPCs in
// AddPlanRequest.Chosen. Otherwise Added lists what landed in the cart.
type AddResult struct {
	Added      []AddedLine  `json:"added"`
	Unresolved []Unresolved `json:"unresolved"`
}

// PlanPreview scales and aggregates a set of recipes into cart lines and staple
// prompts. Pure logic over recipes.json + snoozes.json — no network calls.
func (this *client) PlanPreview(entries []PlanEntry) (*Preview, error) {
	recipes, err := LoadRecipes(recipesPath)
	if err != nil {
		return nil, err
	}
	snoozes, err := LoadSnoozes(snoozesPath)
	if err != nil {
		snoozes = Snoozes{}
	}
	now := time.Now()

	// Aggregate by ingredient name. Keep insertion-stable, dedupe staples,
	// sum package counts across recipes, and remember which recipes each came
	// from for display.
	type agg struct {
		name     string
		packages int
		staple   bool
		scaled   string
		from     []string
	}
	order := []string{}
	byName := map[string]*agg{}
	snoozedSet := map[string]bool{}

	for _, e := range entries {
		r, err := FindRecipe(recipes, e.Recipe)
		if err != nil {
			return nil, err
		}
		want := e.Servings
		if want <= 0 {
			want = r.Servings
		}
		for _, ing := range r.Ingredients {
			base := parsePurchaseQty(ing.Purchase)
			pkgs := scalePackages(base, r.Servings, want)

			a, ok := byName[ing.Name]
			if !ok {
				a = &agg{name: ing.Name, staple: ing.Staple, scaled: scaledQtyHint(ing, r.Servings, want)}
				byName[ing.Name] = a
				order = append(order, ing.Name)
			}
			a.packages += pkgs
			// If the same name is a staple anywhere, treat it as a staple.
			a.staple = a.staple || ing.Staple
			a.from = appendUnique(a.from, r.Name)
		}
	}

	prev := &Preview{}
	for _, name := range order {
		a := byName[name]
		from := strings.Join(a.from, ", ")
		if a.staple {
			if until, ok := snoozes[name]; ok && until.After(now) {
				snoozedSet[name] = true
				continue
			}
			prev.Staples = append(prev.Staples, StaplePrompt{Name: name, Packages: a.packages, From: from})
			continue
		}
		_, hasPreset := this.presets[name]
		prev.Lines = append(prev.Lines, PlanLine{
			Name:      name,
			Packages:  a.packages,
			ScaledQty: a.scaled,
			HasPreset: hasPreset,
			From:      from,
		})
	}
	for name := range snoozedSet {
		prev.Snoozed = append(prev.Snoozed, name)
	}
	sort.Strings(prev.Snoozed)
	return prev, nil
}

// AddPlan resolves every requested line to a UPC and sends them as one batched
// PUT. Resolution order per line: an explicit choice from Chosen (which is also
// pinned as a preset), then an existing preset, then a Kroger search. If any
// line can't be resolved automatically, AddPlan adds NOTHING and returns the
// unresolved items with candidate products for the cook to choose from.
//
// Snoozes are persisted up front (idempotent) so a "no, ask later" sticks even
// if the cart step then bounces back for disambiguation.
func (this *client) AddPlan(ctx context.Context, req AddPlanRequest) (*AddResult, error) {
	if err := this.persistSnoozes(req.Snoozes); err != nil {
		// A failed snooze save shouldn't block the cart — the cook will just be
		// asked again next time. Surface it but keep going.
		fmt.Printf("⚠️ Failed to save snoozes: %v\n", err)
	}

	result := &AddResult{}
	items := make([]CartItem, 0, len(req.Lines))
	presetsDirty := false

	for _, line := range req.Lines {
		name := line.Name
		qty := line.Quantity
		if qty < 1 {
			qty = 1
		}

		// 1) Explicit choice from a prior disambiguation round → pin + use.
		if upc, ok := req.Chosen[name]; ok && strings.TrimSpace(upc) != "" {
			this.presets[name] = upc
			presetsDirty = true
			items = append(items, CartItem{UPC: upc, Quantity: qty, Label: name})
			continue
		}

		// 2) Existing preset → use silently.
		if upc, ok := this.presets[name]; ok {
			items = append(items, CartItem{UPC: upc, Quantity: qty, Label: name})
			continue
		}

		// 3) Search Kroger and offer candidates.
		results, err := this.Search(ctx, name, 5)
		if err != nil {
			result.Unresolved = append(result.Unresolved, Unresolved{
				Name: name,
				Note: fmt.Sprintf("search failed: %v", err),
			})
			continue
		}
		if len(results) == 0 {
			result.Unresolved = append(result.Unresolved, Unresolved{
				Name: name,
				Note: "no matches found — try a different name",
			})
			continue
		}
		cands := make([]Candidate, 0, len(results))
		for _, p := range results {
			cands = append(cands, Candidate{UPC: p.UPC, Display: p.Display()})
		}
		result.Unresolved = append(result.Unresolved, Unresolved{Name: name, Candidates: cands})
	}

	if presetsDirty {
		if err := SavePresets(presetsPath, this.presets); err != nil {
			fmt.Printf("⚠️ Failed to save presets: %v\n", err)
		}
	}

	// Anything unresolved means we don't touch the cart — let the cook decide
	// first, then resubmit. This keeps the add all-or-nothing and avoids a
	// half-filled cart the cook didn't review.
	if len(result.Unresolved) > 0 {
		return result, nil
	}

	if len(items) == 0 {
		return result, nil
	}
	if err := this.putCart(ctx, items); err != nil {
		return nil, err
	}
	for _, it := range items {
		result.Added = append(result.Added, AddedLine{Name: it.Label, UPC: it.UPC, Quantity: it.Quantity})
	}
	return result, nil
}

// persistSnoozes merges new snooze decisions into snoozes.json.
func (this *client) persistSnoozes(decisions []StapleSnooze) error {
	if len(decisions) == 0 {
		return nil
	}
	snoozes, err := LoadSnoozes(snoozesPath)
	if err != nil {
		snoozes = Snoozes{}
	}
	for _, d := range decisions {
		t, err := time.Parse(snoozeDateLayout, strings.TrimSpace(d.Until))
		if err != nil {
			return fmt.Errorf("bad snooze date for %q (%q): %w", d.Name, d.Until, err)
		}
		snoozes[d.Name] = t
	}
	return SaveSnoozes(snoozesPath, snoozes)
}

// scalePackages returns how many purchase-units to buy when a recipe written
// for baseServings is cooked for wantServings. The purchase override is both
// the floor and the unit: a "1 bag" override stays 1 bag for a modest serving
// bump (cooking for 6 instead of 4) and only multiplies when servings cross a
// whole multiple (2 bags at 8 servings). We can't know how many cups are in a
// bag, so fractional bumps round down to the floor; the planner UI lets the
// cook nudge the count if they disagree.
func scalePackages(basePackages, baseServings, wantServings int) int {
	if basePackages < 1 {
		basePackages = 1
	}
	if baseServings < 1 || wantServings < 1 || wantServings <= baseServings {
		return basePackages
	}
	factor := wantServings / baseServings // integer division = floor of the ratio
	if factor < 1 {
		factor = 1
	}
	return basePackages * factor
}

// scaledQtyHint produces a human-readable scaled quantity like "1.5 cups" for
// display only. Returns "" when the recipe has no quantity/unit/servings to
// scale from.
func scaledQtyHint(ing Ingredient, baseServings, wantServings int) string {
	if ing.Quantity <= 0 || ing.Unit == "" || baseServings < 1 || wantServings < 1 {
		return ""
	}
	scaled := ing.Quantity * float64(wantServings) / float64(baseServings)
	return fmt.Sprintf("%s %s", trimFloat(scaled), ing.Unit)
}

// trimFloat formats a float without trailing zeros: 1.5 -> "1.5", 2.0 -> "2".
func trimFloat(f float64) string {
	s := fmt.Sprintf("%.2f", f)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

func appendUnique(xs []string, x string) []string {
	for _, e := range xs {
		if e == x {
			return xs
		}
	}
	return append(xs, x)
}
