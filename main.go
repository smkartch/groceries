package main

import (
	"context"
	"log"
	"time"

	"groceries/kroger"
)

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	token, conf, err := kroger.Authenticate(ctx, "config.json")
	if err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}

	presets, err := kroger.LoadPresets("presets.json")
	if err != nil {
		log.Fatalf("Failed to load presets: %v", err)
	}

	client := kroger.NewClient(ctx, token, conf, presets)

	err = client.AddToCart("milk", 1)
	if err != nil {
		log.Fatalf("Failed to add to cart: %v", err)
	}
}
