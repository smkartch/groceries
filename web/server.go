// Package web serves the cottagecore meal-planner UI on top of the same Kroger
// engine the CLI uses. It is deliberately shape-agnostic: it binds a plain HTTP
// server with embedded static assets and a small JSON API, so it runs the same
// whether it ends up behind a Cloudflare Tunnel, on a tailnet, or just on
// localhost. The hosting decision (Phase 3) lives entirely outside this file.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"groceries/kroger"
)

//go:embed static
var staticFS embed.FS

// Server wires the Kroger engine to HTTP handlers.
type Server struct {
	client kroger.KrogerClient
	mux    *http.ServeMux
}

// NewServer builds the router. The client must already be initialised (auth +
// location + presets loaded) — same lifecycle the CLI uses.
func NewServer(client kroger.KrogerClient) (*Server, error) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("mount static assets: %w", err)
	}

	s := &Server{client: client, mux: http.NewServeMux()}
	s.mux.Handle("GET /", http.FileServer(http.FS(sub)))
	s.mux.HandleFunc("GET /api/recipes", s.handleRecipes)
	s.mux.HandleFunc("POST /api/plan/preview", s.handlePreview)
	s.mux.HandleFunc("POST /api/cart", s.handleCart)
	return s, nil
}

// ListenAndServe starts the server on addr (e.g. ":8090").
func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           logRequests(s.mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("🌿 groceries planner listening on http://localhost%s", addr)
	return srv.ListenAndServe()
}

func (s *Server) handleRecipes(w http.ResponseWriter, r *http.Request) {
	recipes, err := s.client.RecipeList()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if recipes == nil {
		recipes = []kroger.Recipe{}
	}
	writeJSON(w, http.StatusOK, recipes)
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Entries []kroger.PlanEntry `json:"entries"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if len(body.Entries) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no recipes in plan"))
		return
	}
	prev, err := s.client.PlanPreview(body.Entries)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, prev)
}

func (s *Server) handleCart(w http.ResponseWriter, r *http.Request) {
	var req kroger.AddPlanRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	// Cart adds hit Kroger (search + PUT) and may bounce for disambiguation;
	// give it room but don't hang a browser forever.
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	res, err := s.client.AddPlan(ctx, req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// --- helpers ---

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("bad request body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("⚠️ write response: %v", err)
	}
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
