package kroger

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"golang.org/x/oauth2"
)

func newOAuthConfig(cfg Config) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  "http://localhost:8080/callback",
		Scopes:       []string{"product.compact", "cart.basic:write"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://api.kroger.com/v1/connect/oauth2/authorize",
			TokenURL: "https://api.kroger.com/v1/connect/oauth2/token",
		},
	}
}

// AuthURL builds the URL the user should visit in their browser to start the
// OAuth flow. After authorizing, Kroger will redirect them to
// http://localhost:8080/callback?code=... — the `code` value can be passed to
// ExchangeAndSaveCode if they're authing on a different machine than the CLI.
func AuthURL(cfg Config) string {
	return newOAuthConfig(cfg).AuthCodeURL("state-token", oauth2.AccessTypeOffline)
}

// ExchangeAndSaveCode exchanges an auth code (or a full callback URL containing
// a code= query param) for a token and persists it to token.json.
func ExchangeAndSaveCode(ctx context.Context, cfg Config, codeOrURL string) error {
	code, err := extractCode(codeOrURL)
	if err != nil {
		return err
	}
	tok, err := newOAuthConfig(cfg).Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}
	saveToken(tok)
	return nil
}

// extractCode pulls the `code` value out of either a bare code string or a
// full URL like http://localhost:8080/callback?code=...&state=...
func extractCode(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty code")
	}
	if strings.Contains(s, "code=") {
		u, err := url.Parse(s)
		if err == nil {
			if c := u.Query().Get("code"); c != "" {
				return c, nil
			}
		}
		// Fallback: parse it as a raw query string fragment.
		if vals, err := url.ParseQuery(strings.TrimLeft(s, "?")); err == nil {
			if c := vals.Get("code"); c != "" {
				return c, nil
			}
		}
		return "", fmt.Errorf("could not find code= in %q", s)
	}
	return s, nil
}

func Authenticate(ctx context.Context, cfg Config) (*oauth2.Token, *oauth2.Config, error) {
	oauthConf := newOAuthConfig(cfg)

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
		err = fmt.Errorf("unsupported OS %q", runtime.GOOS)
	}
	if err != nil {
		printAuthURL(url)
	}
}

// printAuthURL is for the case where we can't open a browser ourselves.
// Terminal copy-paste of long URLs is fragile (line-wrapping eats the tail
// of the query string), so we also write it to a file as a reliable source.
func printAuthURL(url string) {
	const path = ".kroger-auth-url.txt"
	if err := os.WriteFile(path, []byte(url+"\n"), 0o600); err == nil {
		fmt.Printf("Could not open a browser. The auth URL has been written to %s.\n", path)
		fmt.Printf("Run this in another terminal to copy it cleanly:\n  cat %s\n\n", path)
	}
	fmt.Println("Or copy the URL between the markers below (the whole thing — it WILL wrap):")
	fmt.Println("---BEGIN AUTH URL---")
	fmt.Println(url)
	fmt.Println("---END AUTH URL---")
}
