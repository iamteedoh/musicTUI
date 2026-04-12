package lyrics

import (
	"encoding/json"
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

// Fetch retrieves lyrics from lrclib.net for the given track.
func Fetch(trackName, artistName string, durationSec int) (*Result, error) {
	params := url.Values{
		"track_name":  {trackName},
		"artist_name": {artistName},
	}
	if durationSec > 0 {
		params.Set("duration", strconv.Itoa(durationSec))
	}

	reqURL := "https://lrclib.net/api/get?" + params.Encode()
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "musicTUI/1.0 (https://github.com/iamteedoh/musicTUI)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lrclib request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return &Result{}, nil // no lyrics found
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("lrclib returned status %d", resp.StatusCode)
	}

	var data lrcResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("lrclib parse error: %w", err)
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
