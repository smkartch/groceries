package kroger

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"golang.org/x/oauth2"
)

func Authenticate(ctx context.Context, cfg Config) (*oauth2.Token, *oauth2.Config, error) {
	oauthConf := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  "http://localhost:8080/callback",
		Scopes:       []string{"product.compact", "cart.basic", "cart.modify"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://api.kroger.com/v1/connect/oauth2/authorize",
			TokenURL: "https://api.kroger.com/v1/connect/oauth2/token",
		},
	}

	token, err := loadToken()
	if err == nil && token.Valid() {
		return token, oauthConf, nil
	}

	codeCh := make(chan string)
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		fmt.Fprintf(w, "Authorization received. You may close this window.")
		codeCh <- code
	})

	server := &http.Server{Addr: ":8080"}
	go func() {
		log.Println("Starting local server on http://localhost:8080...")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	url := oauthConf.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	openBrowser(url)
	code := <-codeCh
	server.Shutdown(ctx)

	token, err = oauthConf.Exchange(ctx, code)
	if err != nil {
		return nil, nil, fmt.Errorf("token exchange failed: %w", err)
	}

	saveToken(token)
	return token, oauthConf, nil
}

func loadToken() (*oauth2.Token, error) {
	f, err := os.Open("token.json")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var token oauth2.Token
	err = json.NewDecoder(f).Decode(&token)
	return &token, err
}

func saveToken(token *oauth2.Token) {
	f, err := os.Create("token.json")
	if err != nil {
		log.Printf("Failed to save token: %v", err)
		return
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		fmt.Println("Open this URL manually:", url)
	}
	if err != nil {
		log.Fatalf("Could not open browser: %v", err)
	}
}
