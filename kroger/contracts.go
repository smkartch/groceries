package kroger

import "context"

type Config struct {
	ClientID     string `json:"kroger-client-id"`
	ClientSecret string `json:"kroger-client-secret"`
	LocationID   string `json:"location-id,omitempty"`
}

type KrogerClient interface {
	Init(ctx context.Context, configPath string) error
	AddToCart(ctx context.Context, productID string, quantity int) error
}
