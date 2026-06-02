package kroger

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"golang.org/x/oauth2"
)

type client struct {
	*Config
	token     *oauth2.Token
	oauthConf *oauth2.Config
	presets   Presets
}

func NewClient() KrogerClient {
	return &client{}
}

func (this *client) Init(ctx context.Context, configPath string) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	this.Config = cfg

	this.token, this.oauthConf, err = Authenticate(ctx, *this.Config)
	if err != nil {
		return err
	}

	if cfg.LocationID == "" {
		cfg.LocationID = promptForLocation(ctx, this.token.AccessToken)
		if err := saveConfig(configPath, cfg); err != nil {
			log.Printf("⚠️ Failed to save config with location ID: %v", err)
		}
	}

	this.presets, err = LoadPresets(presetsPath)
	if err != nil {
		return err
	}

	return nil
}

func (this *client) AddToCart(ctx context.Context, itemName string, quantity int) error {
	upc, ok := this.presets[itemName]
	if !ok {
		resolved, err := this.resolve(ctx, itemName)
		if err != nil {
			return err
		}
		upc = resolved
	}

	if err := this.putCart(ctx, []CartItem{{UPC: upc, Quantity: quantity}}); err != nil {
		return err
	}
	fmt.Printf("✅ Added %d × %s to your Kroger cart.\n", quantity, itemName)
	return nil
}

// CartItem is a single line in a batched cart PUT.
type CartItem struct {
	UPC      string
	Quantity int
	// Label is what we print back to the user — e.g. the friendly name, not the UPC.
	Label string
}

// putCart sends one PUT /v1/cart/add request with all items batched together.
// No user-facing output — callers print their own summary.
func (this *client) putCart(ctx context.Context, items []CartItem) error {
	if len(items) == 0 {
		return nil
	}
	payloadItems := make([]map[string]interface{}, len(items))
	for i, it := range items {
		payloadItems[i] = map[string]interface{}{"upc": it.UPC, "quantity": it.Quantity}
	}
	body, err := json.Marshal(map[string]interface{}{"items": payloadItems})
	if err != nil {
		return fmt.Errorf("marshal cart payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, apiBaseURL+"/v1/cart/add", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build cart request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+this.token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send cart request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		var respBody bytes.Buffer
		respBody.ReadFrom(resp.Body)
		return fmt.Errorf("cart PUT returned %d - %s", resp.StatusCode, respBody.String())
	}
	return nil
}

func (this *client) Search(ctx context.Context, term string, limit int) ([]Product, error) {
	return SearchProducts(ctx, this.token.AccessToken, this.LocationID, term, limit)
}

// AddManyToCart resolves a list of {name, quantity} pairs into UPCs (via
// presets or the ask-once flow) and sends them as a single batched PUT. If
// any item fails to resolve, the caller decides whether to abort or skip via
// the per-item OnResolveErr hook; on nil, the default is to abort.
type CartRequest struct {
	Name     string
	Quantity int
}

func (this *client) AddManyToCart(ctx context.Context, reqs []CartRequest) error {
	if len(reqs) == 0 {
		return fmt.Errorf("nothing to add")
	}
	items := make([]CartItem, 0, len(reqs))
	for _, r := range reqs {
		upc, ok := this.presets[r.Name]
		if !ok {
			resolved, err := this.resolve(ctx, r.Name)
			if err != nil {
				return fmt.Errorf("resolve %q: %w", r.Name, err)
			}
			upc = resolved
		}
		items = append(items, CartItem{UPC: upc, Quantity: r.Quantity, Label: r.Name})
	}
	if err := this.putCart(ctx, items); err != nil {
		return err
	}
	for _, it := range items {
		fmt.Printf("✅ Added %d × %s to your Kroger cart.\n", it.Quantity, it.Label)
	}
	return nil
}

func (this *client) ListPresets() Presets {
	return this.presets
}

func (this *client) Pin(itemName, upc string) error {
	itemName = strings.TrimSpace(itemName)
	upc = strings.TrimSpace(upc)
	if itemName == "" || upc == "" {
		return fmt.Errorf("itemName and upc must be non-empty")
	}
	this.presets[itemName] = upc
	return SavePresets(presetsPath, this.presets)
}

func (this *client) Forget(itemName string) error {
	if _, ok := this.presets[itemName]; !ok {
		return fmt.Errorf("no preset for %q", itemName)
	}
	delete(this.presets, itemName)
	return SavePresets(presetsPath, this.presets)
}

// resolve runs the "ask once, remember forever" flow: search Kroger,
// show the top hits, let the user pick one, and persist the choice.
func (this *client) resolve(ctx context.Context, itemName string) (string, error) {
	return this.resolveAs(ctx, itemName, itemName)
}

// resolveAs is resolve with a separate search term and preset key. Callers
// use this when the user re-searches with a different term during cook: we
// search Kroger for the new term but save the result under the recipe's
// original ingredient name, so future cooks of the same recipe hit the preset.
func (this *client) resolveAs(ctx context.Context, presetKey, searchTerm string) (string, error) {
	fmt.Printf("🔎 No preset for %q — searching Kroger…\n", searchTerm)
	results, err := this.Search(ctx, searchTerm, 10)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}
	if len(results) == 0 {
		return "", fmt.Errorf("no products found for %q", searchTerm)
	}

	fmt.Printf("Top matches for %q:\n", searchTerm)
	for i, p := range results {
		fmt.Printf("  [%d] %s\n", i+1, p.Display())
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Pick one [1]: ")
	choiceStr, readErr := reader.ReadString('\n')
	trimmed := strings.TrimSpace(choiceStr)
	// Distinguish "user pressed Enter" (readErr nil, raw line ends in \n) from
	// "stdin was closed before any input" (readErr == io.EOF, empty line).
	// In the latter case the prompt is unanswerable and we must NOT silently
	// pick #1 — it would save the wrong preset and mask the real problem.
	if readErr != nil && trimmed == "" {
		fmt.Println()
		return "", fmt.Errorf("no input received on stdin (running non-interactively?) — run `groceries pin %q <upc>` to set this preset directly", presetKey)
	}
	choice := 1
	if trimmed != "" {
		n, err := strconv.Atoi(trimmed)
		if err != nil || n < 1 || n > len(results) {
			return "", fmt.Errorf("invalid selection %q", trimmed)
		}
		choice = n
	}

	picked := results[choice-1]
	this.presets[presetKey] = picked.UPC
	if err := SavePresets(presetsPath, this.presets); err != nil {
		log.Printf("⚠️ Failed to save preset for %q: %v", presetKey, err)
	} else {
		fmt.Printf("📝 Remembered %q → %s\n", presetKey, picked.UPC)
	}
	return picked.UPC, nil
}

func loadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open config file: %w", err)
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("could not decode config: %w", err)
	}
	return &cfg, nil
}
func saveConfig(path string, cfg *Config) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(cfg)
}

func promptForLocation(ctx context.Context, accessToken string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter your ZIP code: ")
	zip, _ := reader.ReadString('\n')
	zip = strings.TrimSpace(zip)

	locations, err := GetLocationsByZip(ctx, accessToken, zip)
	if err != nil || len(locations) == 0 {
		log.Fatalf("Could not retrieve locations: %v", err)
	}

	fmt.Println("Found the following locations:")
	for i, loc := range locations {
		fmt.Printf("[%d] %s - %s, %s %s (%s)\n", i+1, loc.Name, loc.Address.AddressLine1, loc.Address.City, loc.Address.State, loc.ID)
	}

	fmt.Print("Select a location [1]: ")
	choiceStr, _ := reader.ReadString('\n')
	choiceStr = strings.TrimSpace(choiceStr)
	choice := 0 // default
	fmt.Sscanf(choiceStr, "%d", &choice)

	if choice < 1 || choice > len(locations) {
		choice = 1
	}

	selected := locations[choice-1].ID
	fmt.Printf("✅ Selected location ID: %s\n", selected)
	return selected
}
