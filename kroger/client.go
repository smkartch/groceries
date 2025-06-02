package kroger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
)

type KrogerClient interface {
	AddToCart(productID string, quantity int) error
}

type client struct {
	token     *oauth2.Token
	oauthConf *oauth2.Config
	ctx       context.Context
	presets   Presets
}

// NewClient creates a new client with a valid token
func NewClient(ctx context.Context, token *oauth2.Token, conf *oauth2.Config, presets Presets) KrogerClient {
	return &client{
		token:     token,
		oauthConf: conf,
		ctx:       ctx,
		presets:   presets,
	}
}

const defaultLocationID = "015/00414" // Replace with your Kroger store ID

func (c *client) AddToCart(itemName string, quantity int) error {
	productID, ok := c.presets[itemName]
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
		"locationId": defaultLocationID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(c.ctx, "POST", "https://api.kroger.com/v1/cart/add", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token.AccessToken)
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
