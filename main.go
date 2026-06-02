package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"groceries/kroger"
)

const configPath = "config.json"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "help", "-h", "--help":
		usage()
		return
	case "auth":
		runAuth(args)
		return
	case "auth-url":
		runAuthURL()
		return
	case "add", "search", "list-presets", "forget", "pin", "cook", "recipe":
		// fall through to init + dispatch below
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage()
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := kroger.NewClient()
	if err := client.Init(ctx, configPath); err != nil {
		fail("init: %v", err)
	}

	switch cmd {
	case "add":
		runAdd(ctx, client, args)
	case "search":
		runSearch(ctx, client, args)
	case "list-presets":
		runListPresets(client)
	case "forget":
		runForget(client, args)
	case "pin":
		runPin(client, args)
	case "cook":
		runCook(ctx, client, args)
	case "recipe":
		runRecipe(client, args)
	}
}

func runCook(ctx context.Context, client kroger.KrogerClient, args []string) {
	if len(args) < 1 {
		fail("usage: groceries cook <recipe>")
	}
	if err := client.Cook(ctx, args[0]); err != nil {
		fail("cook: %v", err)
	}
}

func runRecipe(client kroger.KrogerClient, args []string) {
	if len(args) < 1 {
		fail("usage: groceries recipe <list|add|edit> [name]")
	}
	switch args[0] {
	case "list":
		recipes, err := client.RecipeList()
		if err != nil {
			fail("recipe list: %v", err)
		}
		if len(recipes) == 0 {
			fmt.Println("No recipes yet — try `groceries recipe add <name>`.")
			return
		}
		fmt.Println("Recipes:")
		for _, r := range recipes {
			fmt.Printf("  %s (%d ingredients", r.Name, len(r.Ingredients))
			if r.Servings > 0 {
				fmt.Printf(", serves %d", r.Servings)
			}
			fmt.Println(")")
		}
	case "add":
		if len(args) < 2 {
			fail("usage: groceries recipe add <name>")
		}
		if err := client.RecipeAdd(args[1]); err != nil {
			fail("recipe add: %v", err)
		}
	case "edit":
		if len(args) < 2 {
			fail("usage: groceries recipe edit <name>")
		}
		if err := client.RecipeEdit(args[1]); err != nil {
			fail("recipe edit: %v", err)
		}
	default:
		fail("unknown recipe subcommand %q (list|add|edit)", args[0])
	}
}

func runPin(client kroger.KrogerClient, args []string) {
	if len(args) < 2 {
		fail("usage: groceries pin <item> <upc>")
	}
	item, upc := args[0], args[1]
	if err := client.Pin(item, upc); err != nil {
		fail("pin: %v", err)
	}
	fmt.Printf("📝 Pinned %q → %s\n", item, upc)
}

func runForget(client kroger.KrogerClient, args []string) {
	if len(args) < 1 {
		fail("usage: groceries forget <item>")
	}
	item := args[0]
	if err := client.Forget(item); err != nil {
		fail("forget: %v", err)
	}
	fmt.Printf("🗑  Forgot preset for %q. Next `add %s` will re-prompt.\n", item, item)
}

func runAdd(ctx context.Context, client kroger.KrogerClient, args []string) {
	if len(args) < 1 {
		fail("usage: groceries add <item> [qty] [<item> [qty]]…")
	}
	reqs, err := parseAddArgs(args)
	if err != nil {
		fail("add: %v", err)
	}
	if len(reqs) == 1 {
		if err := client.AddToCart(ctx, reqs[0].Name, reqs[0].Quantity); err != nil {
			fail("add: %v", err)
		}
		return
	}
	if err := client.AddManyToCart(ctx, reqs); err != nil {
		fail("add: %v", err)
	}
}

// parseAddArgs interprets the variadic form: each arg is either a new item
// (quantity 1) or — if it parses as a positive integer — a quantity override
// for the previous item. Examples:
//
//	add milk                  → milk×1
//	add milk 2                → milk×2
//	add milk eggs bread       → milk×1, eggs×1, bread×1
//	add milk 2 eggs bread 3   → milk×2, eggs×1, bread×3
func parseAddArgs(args []string) ([]kroger.CartRequest, error) {
	var reqs []kroger.CartRequest
	for _, a := range args {
		if n, err := strconv.Atoi(a); err == nil {
			if n < 1 {
				return nil, fmt.Errorf("quantity must be a positive integer, got %q", a)
			}
			if len(reqs) == 0 {
				return nil, fmt.Errorf("quantity %q has no preceding item", a)
			}
			reqs[len(reqs)-1].Quantity = n
			continue
		}
		reqs = append(reqs, kroger.CartRequest{Name: a, Quantity: 1})
	}
	if len(reqs) == 0 {
		return nil, fmt.Errorf("no items given")
	}
	return reqs, nil
}

func runSearch(ctx context.Context, client kroger.KrogerClient, args []string) {
	if len(args) < 1 {
		fail("usage: groceries search <term>")
	}
	term := args[0]
	results, err := client.Search(ctx, term, 10)
	if err != nil {
		fail("search: %v", err)
	}
	if len(results) == 0 {
		fmt.Printf("No matches for %q.\n", term)
		return
	}
	fmt.Printf("Top matches for %q:\n", term)
	for i, p := range results {
		fmt.Printf("  [%d] %s\n", i+1, p.Display())
	}
}

func runListPresets(client kroger.KrogerClient) {
	presets := client.ListPresets()
	if len(presets) == 0 {
		fmt.Println("No presets yet — add an item to start building your list.")
		return
	}
	names := make([]string, 0, len(presets))
	for name := range presets {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Println("Presets:")
	for _, name := range names {
		fmt.Printf("  %s → %s\n", name, presets[name])
	}
}

func runAuth(args []string) {
	if len(args) < 1 {
		fail("usage: groceries auth <code-or-callback-url>")
	}
	cfg, err := loadConfigForAuth()
	if err != nil {
		fail("auth: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := kroger.ExchangeAndSaveCode(ctx, *cfg, args[0]); err != nil {
		fail("auth: %v", err)
	}
	fmt.Println("✅ Authenticated. Token saved to token.json.")
}

func runAuthURL() {
	cfg, err := loadConfigForAuth()
	if err != nil {
		fail("auth-url: %v", err)
	}
	fmt.Println(kroger.AuthURL(*cfg))
}

func loadConfigForAuth() (*kroger.Config, error) {
	f, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg kroger.Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func usage() {
	fmt.Fprint(os.Stderr, `groceries — small CLI for filling a Kroger cart

Usage:
  groceries add <item> [qty] [<item> [qty]]…   add one or more items (qty defaults to 1; resolves UPCs on first use)
  groceries cook <recipe>           add a recipe's ingredients (prompts for staples; first cook walks new ingredients)
  groceries recipe list             list saved recipes
  groceries recipe add <name>       create a stub recipe and open it in $EDITOR
  groceries recipe edit <name>      edit an existing recipe in $EDITOR
  groceries search <term>           search Kroger products near your store
  groceries list-presets            show the name → UPC map currently saved
  groceries pin <item> <upc>        set a preset directly (e.g. after a manual search) — non-interactive
  groceries forget <item>           remove a preset so next 'add' will re-prompt
  groceries auth-url                print the URL to authorize this app (use when the CLI machine has no browser)
  groceries auth <code-or-url>      exchange an auth code for a token (paste the callback URL or just the code= value)
`)
}

func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "❌ "+format+"\n", args...)
	os.Exit(1)
}
