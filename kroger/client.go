package kroger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

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

	this.presets, err = LoadPresets("presets.json")
	if err != nil {
		return err
	}

	return nil
}

func (this *client) AddToCart(ctx context.Context, itemName string, quantity int) error {
	productID, ok := this.presets[itemName]
	if !ok {
		return fmt.Errorf("no default product ID for item: %s", itemName)
	}

	payload := map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"upc":      productID,
				"quantity": quantity,
			},
		},
		"locationId": this.LocationID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.kroger.com/v1/cart/add", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+this.token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var respBody bytes.Buffer
		respBody.ReadFrom(resp.Body)
		return fmt.Errorf("unexpected response: %d - %s", resp.StatusCode, respBody.String())
	}

	fmt.Printf("✅ Added %d of %s to your Kroger cart.\n", quantity, itemName)
	return nil
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
