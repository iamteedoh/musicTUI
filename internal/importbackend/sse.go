package importbackend

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// ListenEvents tails the SSE stream for a job and pushes typed Event
// values into the returned channel. The channel is closed when the
// stream ends naturally (job_done / error event), the context is
// cancelled, or the connection drops.
//
// Errors that occur while streaming (network drop, malformed frame)
// are surfaced as a final synthetic Event{Type: "stream_error", ...}
// so the caller doesn't need a separate error channel.
func (c *Client) ListenEvents(ctx context.Context, sessionID, jobID string) <-chan Event {
	out := make(chan Event, 64)

	go func() {
		defer close(out)

		req, err := http.NewRequestWithContext(ctx, "GET", c.EventsURL(sessionID, jobID), nil)
		if err != nil {
			out <- streamErr(err)
			return
		}
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cache-Control", "no-cache")

		// SSE needs no read timeout — events arrive whenever the
		// worker emits one. Use a dedicated client without the 90s
		// default timeout the JSON client carries.
		client := &http.Client{Timeout: 0}
		resp, err := client.Do(req)
		if err != nil {
			out <- streamErr(err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			out <- streamErr(&HTTPError{Status: resp.StatusCode, Body: string(body)})
			return
		}

		// Parse line-by-line per the SSE spec. Frames are terminated
		// by a blank line; "event:", "id:" and "data:" build the
		// frame in progress.
		reader := bufio.NewReader(resp.Body)
		var (
			curEvent string
			curID    int
			curData  strings.Builder
		)
		flush := func() {
			if curEvent == "" && curData.Len() == 0 {
				return
			}
			ev := Event{Type: curEvent, Seq: curID}
			if curData.Len() > 0 {
				_ = json.Unmarshal([]byte(curData.String()), &ev.Data)
			}
			select {
			case out <- ev:
			case <-ctx.Done():
			}
			curEvent = ""
			curID = 0
			curData.Reset()
		}

		for {
			if ctx.Err() != nil {
				return
			}
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF && (curEvent != "" || curData.Len() > 0) {
					flush()
				}
				if err != io.EOF {
					out <- streamErr(err)
				}
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				flush()
				// If the just-flushed event is a terminal one, close.
				continue
			}
			switch {
			case strings.HasPrefix(line, "event:"):
				curEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "id:"):
				if n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "id:"))); err == nil {
					curID = n
				}
			case strings.HasPrefix(line, "data:"):
				curData.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			case strings.HasPrefix(line, ":"):
				// SSE comment / keep-alive — ignore.
			}
		}
	}()

	return out
}

func streamErr(err error) Event {
	return Event{
		Type: "stream_error",
		Data: map[string]any{"message": fmt.Sprintf("%v", err)},
	}
}
