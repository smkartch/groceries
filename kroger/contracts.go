package kroger

import "context"

type Config struct {
	ClientID     string `json:"kroger-client-id"`
	ClientSecret string `json:"kroger-client-secret"`
	LocationID   string `json:"location-id,omitempty"`
}

type KrogerClient interface {
	Init(ctx context.Context, configPath string) error
	AddToCart(ctx context.Context, itemName string, quantity int) error
	AddManyToCart(ctx context.Context, reqs []CartRequest) error
	Cook(ctx context.Context, recipeName string) error
	RecipeList() ([]Recipe, error)
	RecipeAdd(name string) error
	RecipeEdit(name string) error
	Search(ctx context.Context, term string, limit int) ([]Product, error)
	ListPresets() Presets
	Pin(itemName, upc string) error
	Forget(itemName string) error
}
