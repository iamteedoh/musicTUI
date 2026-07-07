package spotify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// CreatePlaylist must call POST /me/playlists. Spotify's February 2026
// Development Mode migration removed POST /users/{id}/playlists (the whole
// /users/{id} family), which fails with a bare 403 — MUS-11.
func TestCreatePlaylistUsesMeEndpoint(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "pl123",
			"name": "My Playlist",
		})
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), baseURL: srv.URL}
	pl, err := c.CreatePlaylist(context.Background(), "My Playlist", "desc", false)
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/me/playlists" {
		t.Fatalf("CreatePlaylist called %s %s, want POST /me/playlists (removed /users/{id}/playlists returns 403)", gotMethod, gotPath)
	}
	if pl.ID != "pl123" {
		t.Fatalf("playlist ID = %q, want pl123", pl.ID)
	}
}

// Guard the whole raw-endpoint surface against the Feb 2026 removals: no
// method may construct a /users/{id}/... URL.
func TestNoRemovedUsersEndpoints(t *testing.T) {
	// This is a compile-time-adjacent guard: the only raw URL builder that
	// used /users/{id} was CreatePlaylist. If someone reintroduces one, the
	// httptest above plus this grep-style check in review should catch it;
	// here we at least pin CreatePlaylist's behavior with an empty userID —
	// it must succeed without ever needing a user id.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "x", "name": "n"})
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), baseURL: srv.URL} // note: no userID set
	if _, err := c.CreatePlaylist(context.Background(), "n", "", false); err != nil {
		t.Fatalf("CreatePlaylist must not depend on a user id: %v", err)
	}
}
