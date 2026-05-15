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

	payload := map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"upc":      upc,
				"quantity": quantity,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, apiBaseURL+"/v1/cart/add", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+this.token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		var respBody bytes.Buffer
		respBody.ReadFrom(resp.Body)
		return fmt.Errorf("unexpected response: %d - %s", resp.StatusCode, respBody.String())
	}

	fmt.Printf("✅ Added %d × %s to your Kroger cart.\n", quantity, itemName)
	return nil
}

func (this *client) Search(ctx context.Context, term string, limit int) ([]Product, error) {
	return SearchProducts(ctx, this.token.AccessToken, this.LocationID, term, limit)
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
	fmt.Printf("🔎 No preset for %q — searching Kroger…\n", itemName)
	results, err := this.Search(ctx, itemName, 10)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}
	if len(results) == 0 {
		return "", fmt.Errorf("no products found for %q", itemName)
	}

	fmt.Printf("Top matches for %q:\n", itemName)
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
		return "", fmt.Errorf("no input received on stdin (running non-interactively?) — run `groceries pin %q <upc>` to set this preset directly", itemName)
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
	this.presets[itemName] = picked.UPC
	if err := SavePresets(presetsPath, this.presets); err != nil {
		log.Printf("⚠️ Failed to save preset for %q: %v", itemName, err)
	} else {
		fmt.Printf("📝 Remembered %q → %s\n", itemName, picked.UPC)
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
