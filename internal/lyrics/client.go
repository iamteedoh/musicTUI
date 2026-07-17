package lyrics

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// LyricLine represents a single timestamped lyric line.
type LyricLine struct {
	TimeMs int64
	Text   string
}

// Result holds fetched lyrics data.
type Result struct {
	Lines  []LyricLine // synced lyrics (timestamped)
	Plain  string      // plain text fallback
	Synced bool        // true if Lines is populated
}

// lrcResponse is the JSON response from lrclib.net.
type lrcResponse struct {
	SyncedLyrics string `json:"syncedLyrics"`
	PlainLyrics  string `json:"plainLyrics"`
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

// baseURL is a package var so tests can point Fetch at a local server.
var baseURL = "https://lrclib.net"

// retryBackoff is the pause before Fetch's single silent retry of a
// transient failure. Package var so tests can zero it.
var retryBackoff = 2 * time.Second

// ErrKind classifies a lyrics fetch failure for user-facing display.
type ErrKind int

const (
	// ErrNetwork: the request never completed (DNS, refused, timeout).
	ErrNetwork ErrKind = iota
	// ErrServiceBusy: lrclib answered 429 or a 5xx — usually transient.
	ErrServiceBusy
	// ErrBadResponse: unexpected status or an unparseable body.
	ErrBadResponse
)

// FetchError wraps a lyrics fetch failure with a classification the UI can
// map to a short plain-language message. The wrapped error keeps the raw
// detail (URL, query string, transport error) available for debugging while
// keeping it out of the panel (MUS-33).
type FetchError struct {
	Kind ErrKind
	Err  error
}

func (e *FetchError) Error() string { return e.Err.Error() }
func (e *FetchError) Unwrap() error { return e.Err }

// UserMessage maps a Fetch failure to a short message safe to render in the
// TUI: no URLs, no query strings, no raw Go error chains (MUS-33).
func UserMessage(err error) string {
	var fe *FetchError
	if errors.As(err, &fe) {
		switch fe.Kind {
		case ErrNetwork:
			return "Couldn't reach the lyrics service"
		case ErrServiceBusy:
			return "The lyrics service is busy, try again shortly"
		}
	}
	return "Couldn't load lyrics for this track"
}

// Fetch retrieves lyrics from lrclib.net for the given track. Transient
// failures (network, rate limit, server error) are retried once after a
// short backoff before being reported — most of them clear on their own.
func Fetch(trackName, artistName string, durationSec int) (*Result, error) {
	result, err := fetchOnce(trackName, artistName, durationSec)
	if isTransient(err) {
		time.Sleep(retryBackoff)
		result, err = fetchOnce(trackName, artistName, durationSec)
	}
	return result, err
}

func isTransient(err error) bool {
	var fe *FetchError
	return errors.As(err, &fe) && (fe.Kind == ErrNetwork || fe.Kind == ErrServiceBusy)
}

func fetchOnce(trackName, artistName string, durationSec int) (*Result, error) {
	params := url.Values{
		"track_name":  {trackName},
		"artist_name": {artistName},
	}
	if durationSec > 0 {
		params.Set("duration", strconv.Itoa(durationSec))
	}

	reqURL := baseURL + "/api/get?" + params.Encode()
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, &FetchError{Kind: ErrBadResponse, Err: err}
	}
	req.Header.Set("User-Agent", "musicTUI/1.0 (https://github.com/iamteedoh/musicTUI)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, &FetchError{Kind: ErrNetwork, Err: fmt.Errorf("lrclib request failed: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return &Result{}, nil // no lyrics found
	}
	if resp.StatusCode != 200 {
		kind := ErrBadResponse
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			kind = ErrServiceBusy
		}
		return nil, &FetchError{Kind: kind, Err: fmt.Errorf("lrclib returned status %d", resp.StatusCode)}
	}

	var data lrcResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, &FetchError{Kind: ErrBadResponse, Err: fmt.Errorf("lrclib parse error: %w", err)}
	}

	result := &Result{Plain: data.PlainLyrics}

	if data.SyncedLyrics != "" {
		result.Lines = parseLRC(data.SyncedLyrics)
		result.Synced = len(result.Lines) > 0
	}

	return result, nil
}

// parseLRC parses LRC format: [mm:ss.xx] text
func parseLRC(lrc string) []LyricLine {
	var lines []LyricLine
	for _, raw := range strings.Split(lrc, "\n") {
		raw = strings.TrimSpace(raw)
		if len(raw) < 10 || raw[0] != '[' {
			continue
		}
		closeBracket := strings.Index(raw, "]")
		if closeBracket < 0 {
			continue
		}

		timestamp := raw[1:closeBracket]
		text := strings.TrimSpace(raw[closeBracket+1:])

		parts := strings.Split(timestamp, ":")
		if len(parts) != 2 {
			continue
		}

		min, err1 := strconv.Atoi(parts[0])
		secParts := strings.Split(parts[1], ".")
		sec, err2 := strconv.Atoi(secParts[0])
		ms := 0
		if len(secParts) > 1 {
			msStr := secParts[1]
			if len(msStr) == 2 {
				msStr += "0" // convert centiseconds to milliseconds
			}
			ms, _ = strconv.Atoi(msStr)
		}

		if err1 != nil || err2 != nil {
			continue
		}

		timeMs := int64(min)*60000 + int64(sec)*1000 + int64(ms)
		lines = append(lines, LyricLine{TimeMs: timeMs, Text: text})
	}
	return lines
}
