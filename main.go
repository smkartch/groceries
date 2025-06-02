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

	client := kroger.NewClient()
	if err := client.Init(ctx, "config.json"); err != nil {
		log.Fatalf("Failed to initialize client: %v", err)
	}

	err := client.AddToCart(ctx, "milk", 1)
	if err != nil {
		log.Fatalf("Failed to add to cart: %v", err)
	}
}
