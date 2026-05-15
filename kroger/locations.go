package kroger

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

type Location struct {
	ID      string `json:"locationId"`
	Name    string `json:"name"`
	Address struct {
		AddressLine1 string `json:"addressLine1"`
		City         string `json:"city"`
		State        string `json:"state"`
		ZipCode      string `json:"zipCode"`
	} `json:"address"`
}

type locationResponse struct {
	Data []Location `json:"data"`
}

func GetLocationsByZip(ctx context.Context, token string, zip string) ([]Location, error) {
	u := apiBaseURL + "/v1/locations"
	params := url.Values{}
	params.Set("filter.zipCode.near", zip)
	params.Set("filter.locationType", "store")
	fullURL := u + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var locResp locationResponse
	if err := json.NewDecoder(resp.Body).Decode(&locResp); err != nil {
		return nil, err
	}

	return locResp.Data, nil
}
