package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/iamteedoh/musicTUI/internal/model"
)

// PlaylistBackup is a snapshot of a single playlist sufficient to restore it,
// either by re-following the original ID or by recreating it from scratch.
type PlaylistBackup struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Owner       string   `json:"owner"`
	TrackURIs   []string `json:"track_uris"`
}

// BackupFile is a timestamped collection of playlist snapshots written before
// a destructive operation (merge/unfollow).
type BackupFile struct {
	CreatedAt string           `json:"created_at"`
	Reason    string           `json:"reason"`
	Playlists []PlaylistBackup `json:"playlists"`
}

// BackupDir returns the directory where playlist backups are stored,
// creating it if necessary.
func BackupDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "musicTUI", "playlist-backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// allTrackURIs pages through a playlist and returns every track URI.
func (c *Client) allTrackURIs(ctx context.Context, playlistID string) []string {
	var uris []string
	offset := 0
	for {
		page, err := c.GetPlaylistTracks(ctx, playlistID, offset, maxPageSize)
		if err != nil {
			break
		}
		for _, t := range page.Items {
			if t.URI != "" {
				uris = append(uris, t.URI)
			}
		}
		if uint32(offset+len(page.Items)) >= page.Total || len(page.Items) == 0 {
			break
		}
		offset += len(page.Items)
	}
	return uris
}

// SnapshotPlaylists captures the given playlists (including their full track
// lists) to a timestamped JSON file before they are unfollowed, so they can be
// restored in-app afterwards. It returns the path of the file written.
func (c *Client) SnapshotPlaylists(ctx context.Context, playlists []model.Playlist, reason string) (string, error) {
	if len(playlists) == 0 {
		return "", nil
	}
	dir, err := BackupDir()
	if err != nil {
		return "", err
	}

	snap := BackupFile{
		CreatedAt: time.Now().Format(time.RFC3339),
		Reason:    reason,
	}
	for _, pl := range playlists {
		snap.Playlists = append(snap.Playlists, PlaylistBackup{
			ID:          pl.ID,
			Name:        pl.Name,
			Description: pl.Description,
			Owner:       pl.Owner,
			TrackURIs:   c.allTrackURIs(ctx, pl.ID),
		})
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", err
	}
	// Filename is sortable by time so LoadLatestBackup can pick the newest.
	path := filepath.Join(dir, fmt.Sprintf("backup-%s.json", time.Now().Format("20060102-150405")))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ListBackups returns the paths of all backup files, newest first.
func ListBackups() ([]string, error) {
	dir, err := BackupDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	// Names are timestamped, so reverse-lexical == newest first.
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	return paths, nil
}

// HasBackups reports whether at least one playlist backup exists on disk.
func HasBackups() bool {
	paths, err := ListBackups()
	return err == nil && len(paths) > 0
}

// LoadBackup reads and parses a single backup file.
func LoadBackup(path string) (*BackupFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bf BackupFile
	if err := json.Unmarshal(data, &bf); err != nil {
		return nil, err
	}
	return &bf, nil
}

// LoadLatestBackup returns the most recent backup and its path, or an error if
// none exist.
func LoadLatestBackup() (*BackupFile, string, error) {
	paths, err := ListBackups()
	if err != nil {
		return nil, "", err
	}
	if len(paths) == 0 {
		return nil, "", fmt.Errorf("no playlist backups found")
	}
	bf, err := LoadBackup(paths[0])
	if err != nil {
		return nil, "", err
	}
	return bf, paths[0], nil
}

// RestoreFromBackup restores every playlist in the backup. It first tries to
// re-follow the original playlist by ID (cheap, perfect fidelity); if that
// fails (the playlist was hard-deleted/garbage-collected) it recreates the
// playlist and re-adds the backed-up tracks. Returns counts of (refollowed,
// recreated, failed).
func (c *Client) RestoreFromBackup(ctx context.Context, bf *BackupFile) (refollowed, recreated, failed int) {
	for _, pl := range bf.Playlists {
		if pl.ID != "" {
			if err := c.FollowPlaylist(ctx, pl.ID); err == nil {
				refollowed++
				continue
			}
		}
		// Fall back to recreating the playlist from the snapshot.
		newPl, err := c.CreatePlaylist(ctx, pl.Name, pl.Description, false)
		if err != nil {
			failed++
			continue
		}
		for i := 0; i < len(pl.TrackURIs); i += 100 {
			end := i + 100
			if end > len(pl.TrackURIs) {
				end = len(pl.TrackURIs)
			}
			if err := c.AddTracksToPlaylist(ctx, newPl.ID, pl.TrackURIs[i:end]); err != nil {
				break
			}
		}
		recreated++
	}
	return refollowed, recreated, failed
}
