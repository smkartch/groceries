package kroger

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

type Product struct {
	ProductID   string        `json:"productId"`
	UPC         string        `json:"upc"`
	Brand       string        `json:"brand"`
	Description string        `json:"description"`
	Items       []ProductItem `json:"items"`
}

type ProductItem struct {
	ItemID string `json:"itemId"`
	Size   string `json:"size"`
	Price  struct {
		Regular float64 `json:"regular"`
		Promo   float64 `json:"promo"`
	} `json:"price"`
}

type productsResponse struct {
	Data []Product `json:"data"`
}

// Display returns a one-line label suitable for a picker.
func (p Product) Display() string {
	size := ""
	price := ""
	if len(p.Items) > 0 {
		item := p.Items[0]
		if item.Size != "" {
			size = " · " + item.Size
		}
		switch {
		case item.Price.Promo > 0 && item.Price.Promo < item.Price.Regular:
			price = fmt.Sprintf(" · $%.2f (was $%.2f)", item.Price.Promo, item.Price.Regular)
		case item.Price.Regular > 0:
			price = fmt.Sprintf(" · $%.2f", item.Price.Regular)
		}
	}
	brand := ""
	if p.Brand != "" {
		brand = p.Brand + " "
	}
	return fmt.Sprintf("%s%s%s%s [%s]", brand, p.Description, size, price, p.UPC)
}

func SearchProducts(ctx context.Context, token, locationID, term string, limit int) ([]Product, error) {
	if limit <= 0 {
		limit = 10
	}

	params := url.Values{}
	params.Set("filter.term", term)
	if locationID != "" {
		params.Set("filter.locationId", locationID)
	}
	params.Set("filter.limit", strconv.Itoa(limit))

	req, err := http.NewRequestWithContext(ctx, "GET", apiBaseURL+"/v1/products?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("product search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("product search returned %d", resp.StatusCode)
	}

	var out productsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding product search response: %w", err)
	}
	return out.Data, nil
}
