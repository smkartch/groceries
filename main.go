package main

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

// Config holds the credentials loaded from config.json
type Config struct {
	ClientID     string `json:"kroger-client-id"`
	ClientSecret string `json:"kroger-client-secret"`
}

var (
	redirectURI = "http://localhost:8080/callback"
	authURL     = "https://api.kroger.com/v1/connect/oauth2/authorize"
	tokenURL    = "https://api.kroger.com/v1/connect/oauth2/token"
	scopes      = []string{"product.compact", "cart.basic:write"}

	tokenFile  = "token.json"
	configFile = "config.json"
)

func main() {
	ctx := context.Background()

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	oauthConf := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  redirectURI,
		Scopes:       scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  authURL,
			TokenURL: tokenURL,
		},
	}

	// Try to load saved token
	token, err := loadToken()
	if err == nil && token.Valid() {
		fmt.Println("Access token loaded from file.")
	} else {
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
				log.Fatalf("Server failed: %v", err)
			}
		}()

		url := oauthConf.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
		fmt.Println("Opening browser for authentication...")
		openBrowser(url)

		code := <-codeCh
		server.Shutdown(ctx)

		token, err = oauthConf.Exchange(ctx, code)
		if err != nil {
			log.Fatalf("Token exchange failed: %v", err)
		}

		if err := saveToken(token); err != nil {
			log.Printf("Warning: failed to save token: %v", err)
		}
	}

	fmt.Println("Authorized! Now to the fun part...")
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
		fmt.Println("Please open the following URL manually:", url)
	}
	if err != nil {
		log.Fatalf("Failed to open browser: %v", err)
	}
}

func saveToken(token *oauth2.Token) error {
	f, err := os.Create(tokenFile)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(token)
}
func loadToken() (*oauth2.Token, error) {
	f, err := os.Open(tokenFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var token oauth2.Token
	err = json.NewDecoder(f).Decode(&token)
	return &token, err
}

func loadConfig() (*Config, error) {
	f, err := os.Open(configFile)
	if err != nil {
		return nil, fmt.Errorf("could not open config file: %w", err)
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("could not decode config JSON: %w", err)
	}

	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("client ID or client secret missing in config")
	}

	return &cfg, nil
}
