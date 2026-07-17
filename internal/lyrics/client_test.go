package lyrics

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// pointFetchAt routes Fetch at a local handler with the retry backoff zeroed,
// restoring both after the test. Returns a counter of requests received so
// tests can assert on retry behavior.
func pointFetchAt(t *testing.T, handler http.HandlerFunc) *int {
	t.Helper()
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	oldURL, oldBackoff := baseURL, retryBackoff
	baseURL, retryBackoff = srv.URL, 0
	t.Cleanup(func() { baseURL, retryBackoff = oldURL, oldBackoff })
	return &requests
}

// Every failure path must classify into a FetchError kind the UI can map to
// a plain-language message. The raw lrclib detail (URL, status, transport
// error) stays wrapped inside — it must never be the user-facing text
// (MUS-33).
func TestFetchClassifiesFailures(t *testing.T) {
	cases := []struct {
		name     string
		handler  http.HandlerFunc
		wantKind ErrKind
		// transient kinds get one silent retry, so the server sees 2 requests
		wantRequests int
	}{
		{
			name:         "rate limited",
			handler:      func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429) },
			wantKind:     ErrServiceBusy,
			wantRequests: 2,
		},
		{
			name:         "server error",
			handler:      func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) },
			wantKind:     ErrServiceBusy,
			wantRequests: 2,
		},
		{
			name:         "unexpected client status",
			handler:      func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) },
			wantKind:     ErrBadResponse,
			wantRequests: 1,
		},
		{
			name: "unparseable body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("<html>not json</html>"))
			},
			wantKind:     ErrBadResponse,
			wantRequests: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := pointFetchAt(t, tc.handler)
			_, err := Fetch("Song", "Artist", 200)
			var fe *FetchError
			if !errors.As(err, &fe) {
				t.Fatalf("Fetch error = %v (%T), want *FetchError", err, err)
			}
			if fe.Kind != tc.wantKind {
				t.Fatalf("Kind = %v, want %v", fe.Kind, tc.wantKind)
			}
			if *requests != tc.wantRequests {
				t.Fatalf("server saw %d requests, want %d", *requests, tc.wantRequests)
			}
		})
	}
}

// A dead network (connection refused) is ErrNetwork — and transient, so it
// is attempted twice.
func TestFetchNetworkFailureIsErrNetwork(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	deadURL := srv.URL
	srv.Close()
	oldURL, oldBackoff := baseURL, retryBackoff
	baseURL, retryBackoff = deadURL, 0
	t.Cleanup(func() { baseURL, retryBackoff = oldURL, oldBackoff })

	_, err := Fetch("Song", "Artist", 200)
	var fe *FetchError
	if !errors.As(err, &fe) {
		t.Fatalf("Fetch error = %v (%T), want *FetchError", err, err)
	}
	if fe.Kind != ErrNetwork {
		t.Fatalf("Kind = %v, want ErrNetwork", fe.Kind)
	}
}

// A transient failure that clears on the silent retry must surface as
// success — the user never sees the blip.
func TestFetchRetriesTransientFailureSilently(t *testing.T) {
	failed := false
	requests := pointFetchAt(t, func(w http.ResponseWriter, r *http.Request) {
		if !failed {
			failed = true
			w.WriteHeader(429)
			return
		}
		w.Write([]byte(`{"syncedLyrics": "", "plainLyrics": "la la la"}`))
	})

	result, err := Fetch("Song", "Artist", 200)
	if err != nil {
		t.Fatalf("Fetch after transient blip = %v, want success", err)
	}
	if result.Plain != "la la la" {
		t.Fatalf("Plain = %q, want the retried body", result.Plain)
	}
	if *requests != 2 {
		t.Fatalf("server saw %d requests, want 2 (fail, then silent retry)", *requests)
	}
}

// 404 means "no lyrics for this track" — an empty result and a silent panel,
// never an error banner.
func TestFetch404MeansNoLyricsNotAnError(t *testing.T) {
	pointFetchAt(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	result, err := Fetch("Song", "Artist", 200)
	if err != nil {
		t.Fatalf("404 produced error %v, want nil", err)
	}
	if result == nil || result.Synced || result.Plain != "" {
		t.Fatalf("404 result = %+v, want empty Result", result)
	}
}

// The user-facing strings must be plain language: no endpoint, no query
// string, no wrapped Go error text.
func TestUserMessageIsCleanPlainLanguage(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&FetchError{Kind: ErrNetwork, Err: errors.New(`Get "https://lrclib.net/api/get?artist_name=X": dial tcp: timeout`)}, "Couldn't reach the lyrics service"},
		{&FetchError{Kind: ErrServiceBusy, Err: errors.New("lrclib returned status 429")}, "The lyrics service is busy, try again shortly"},
		{&FetchError{Kind: ErrBadResponse, Err: errors.New("lrclib parse error: invalid character '<'")}, "Couldn't load lyrics for this track"},
		{errors.New("some unwrapped error"), "Couldn't load lyrics for this track"},
	}
	for _, tc := range cases {
		got := UserMessage(tc.err)
		if got != tc.want {
			t.Errorf("UserMessage(%v) = %q, want %q", tc.err, got, tc.want)
		}
		for _, leak := range []string{"lrclib", "http", "%", "api"} {
			if strings.Contains(strings.ToLower(got), leak) {
				t.Errorf("UserMessage(%v) = %q leaks %q", tc.err, got, leak)
			}
		}
	}
}
