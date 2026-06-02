package kroger

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Cook runs a recipe: walks ingredients, fills in any missing fields, prompts
// per-staple "running low?", resolves UPCs (with per-failure skip/retry/abort),
// and sends every cart-bound item in one batched PUT.
func (this *client) Cook(ctx context.Context, recipeName string) error {
	recipes, err := LoadRecipes(recipesPath)
	if err != nil {
		return err
	}
	r, err := FindRecipe(recipes, recipeName)
	if err != nil {
		return err
	}

	snoozes, err := LoadSnoozes(snoozesPath)
	if err != nil {
		log.Printf("⚠️ Could not load snoozes (%v) — proceeding without them.", err)
		snoozes = Snoozes{}
	}

	reader := bufio.NewReader(os.Stdin)

	// Pass 1: walk-through any ingredient missing the fields we need.
	// Save back to disk only if we actually changed something.
	dirty := false
	for i := range r.Ingredients {
		ing := &r.Ingredients[i]
		if needsWalkthrough(*ing) {
			if err := walkthroughIngredient(reader, r.Name, ing); err != nil {
				return err
			}
			dirty = true
		}
	}
	if dirty {
		if err := SaveRecipes(recipesPath, recipes); err != nil {
			return fmt.Errorf("save recipes after walk-through: %w", err)
		}
	}

	// Pass 2: classify each ingredient and gather what we'll actually add.
	now := time.Now()
	var batch []CartRequest
	snoozesDirty := false
	for _, ing := range r.Ingredients {
		if ing.Staple {
			if until, ok := snoozes[ing.Name]; ok && until.After(now) {
				fmt.Printf("⏰ Skipping %s (snoozed until %s).\n", ing.Name, until.Format(snoozeDateLayout))
				continue
			}
			ans, until, err := promptStaple(reader, ing.Name)
			if err != nil {
				return err
			}
			switch ans {
			case stapleYes:
				batch = append(batch, CartRequest{Name: ing.Name, Quantity: parsePurchaseQty(ing.Purchase)})
			case stapleNo:
				// fall through — don't buy, don't snooze
			case stapleSnooze:
				snoozes[ing.Name] = until
				snoozesDirty = true
				fmt.Printf("💤 Won't ask about %s again until %s.\n", ing.Name, until.Format(snoozeDateLayout))
			}
		} else {
			batch = append(batch, CartRequest{Name: ing.Name, Quantity: parsePurchaseQty(ing.Purchase)})
		}
	}
	if snoozesDirty {
		if err := SaveSnoozes(snoozesPath, snoozes); err != nil {
			log.Printf("⚠️ Failed to save snoozes: %v", err)
		}
	}

	// Pass 3: resolve UPCs (preset or ask-once) for everything in the batch,
	// with skip/retry/abort on resolve failures.
	items, err := this.resolveBatch(ctx, reader, batch)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Println("Nothing to add to the cart.")
		return nil
	}

	if err := this.putCart(ctx, items); err != nil {
		return err
	}
	for _, it := range items {
		fmt.Printf("✅ Added %d × %s.\n", it.Quantity, it.Label)
	}
	fmt.Printf("🍳 %s — %d items added.\n", r.Name, len(items))
	return nil
}

// needsWalkthrough reports whether we have enough info to act on this
// ingredient. We use Purchase as the sentinel: if empty, the user hasn't
// configured this ingredient yet, so ask about it (including staple flag).
// Once Purchase is non-empty, we trust Staple's value (zero = non-staple).
func needsWalkthrough(ing Ingredient) bool {
	return strings.TrimSpace(ing.Purchase) == "" && !ing.Staple
}

func walkthroughIngredient(reader *bufio.Reader, recipeName string, ing *Ingredient) error {
	fmt.Printf("\n🌿 %s — %s needs a one-time setup.\n", recipeName, ing.Name)

	isStaple, err := promptYesNo(reader, "   Is this a staple (always-on-hand spices/oil/etc.)?", false)
	if err != nil {
		return err
	}
	ing.Staple = isStaple

	purchase, err := promptLine(reader, `   How much to buy when needed? (e.g. "1 package", "2 lbs")`, "1 package")
	if err != nil {
		return err
	}
	ing.Purchase = purchase
	fmt.Println("   Saved.")
	return nil
}

type stapleAnswer int

const (
	stapleNo stapleAnswer = iota
	stapleYes
	stapleSnooze
)

func promptStaple(reader *bufio.Reader, name string) (stapleAnswer, time.Time, error) {
	for {
		fmt.Printf("🧂 Running low on %s? [y]es / [N]o / [s]nooze: ", name)
		raw, err := reader.ReadString('\n')
		if err != nil && strings.TrimSpace(raw) == "" {
			return stapleNo, time.Time{}, fmt.Errorf("no input on stdin while prompting about %q", name)
		}
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "y", "yes":
			return stapleYes, time.Time{}, nil
		case "", "n", "no":
			return stapleNo, time.Time{}, nil
		case "s", "snooze":
			fmt.Print("   Snooze until (YYYY-MM-DD): ")
			dateRaw, err := reader.ReadString('\n')
			if err != nil && strings.TrimSpace(dateRaw) == "" {
				return stapleNo, time.Time{}, fmt.Errorf("no snooze date entered")
			}
			t, err := time.Parse(snoozeDateLayout, strings.TrimSpace(dateRaw))
			if err != nil {
				fmt.Println("   Not a valid date — let's try the whole question again.")
				continue
			}
			return stapleSnooze, t, nil
		default:
			fmt.Println("   Didn't understand — y / n / s please.")
		}
	}
}

// resolveBatch walks a CartRequest list and turns each into a CartItem.
// On resolve failure it prompts: skip / retry-with-new-term / abort.
// presetKey stays the recipe's original ingredient name across retries, so
// the preset is saved under that name and next cook of this recipe hits it.
func (this *client) resolveBatch(ctx context.Context, reader *bufio.Reader, reqs []CartRequest) ([]CartItem, error) {
	items := make([]CartItem, 0, len(reqs))
	for _, req := range reqs {
		presetKey := req.Name
		searchTerm := req.Name
		for {
			upc, ok := this.presets[presetKey]
			if !ok {
				resolved, rerr := this.resolveAs(ctx, presetKey, searchTerm)
				if rerr == nil {
					upc = resolved
				} else {
					action, newTerm, perr := promptResolveFail(reader, presetKey, rerr)
					if perr != nil {
						return nil, perr
					}
					switch action {
					case resolveSkip:
						fmt.Printf("⏭  Skipping %s.\n", presetKey)
						goto next
					case resolveAbort:
						return nil, fmt.Errorf("aborted on %q: %w", presetKey, rerr)
					case resolveRetry:
						searchTerm = newTerm
						continue
					}
				}
			}
			items = append(items, CartItem{UPC: upc, Quantity: req.Quantity, Label: presetKey})
			break
		}
	next:
	}
	return items, nil
}

type resolveAction int

const (
	resolveSkip resolveAction = iota
	resolveRetry
	resolveAbort
)

func promptResolveFail(reader *bufio.Reader, name string, cause error) (resolveAction, string, error) {
	fmt.Printf("⚠️  Couldn't resolve %q: %v\n", name, cause)
	for {
		fmt.Print("   [s]kip / [r]e-search with different term / [a]bort: ")
		raw, err := reader.ReadString('\n')
		if err != nil && strings.TrimSpace(raw) == "" {
			return resolveAbort, "", fmt.Errorf("no input on stdin during failure prompt")
		}
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "s", "skip":
			return resolveSkip, "", nil
		case "a", "abort":
			return resolveAbort, "", nil
		case "r", "retry", "re-search":
			fmt.Print("   New search term: ")
			term, err := reader.ReadString('\n')
			if err != nil && strings.TrimSpace(term) == "" {
				return resolveAbort, "", fmt.Errorf("no replacement term entered")
			}
			term = strings.TrimSpace(term)
			if term == "" {
				fmt.Println("   Empty term — try again.")
				continue
			}
			return resolveRetry, term, nil
		default:
			fmt.Println("   s / r / a please.")
		}
	}
}

func promptYesNo(reader *bufio.Reader, prompt string, defaultYes bool) (bool, error) {
	suffix := " [y/N]: "
	if defaultYes {
		suffix = " [Y/n]: "
	}
	fmt.Print(prompt + suffix)
	raw, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(raw) == "" && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	case "":
		return defaultYes, nil
	default:
		return false, nil
	}
}

func promptLine(reader *bufio.Reader, prompt, fallback string) (string, error) {
	fmt.Printf("%s [%s]: ", prompt, fallback)
	raw, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("no input on stdin")
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback, nil
	}
	return trimmed, nil
}

var leadingInt = regexp.MustCompile(`^\s*(\d+)`)

// parsePurchaseQty pulls the leading integer out of strings like "1 bag" or
// "2 lbs". Defaults to 1 when the string is empty or has no leading number —
// the cart needs a positive count even if the purchase override is vague.
func parsePurchaseQty(purchase string) int {
	m := leadingInt.FindStringSubmatch(purchase)
	if m == nil {
		return 1
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 {
		return 1
	}
	return n
}
