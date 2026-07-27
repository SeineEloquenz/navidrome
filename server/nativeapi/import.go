package nativeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	playlistsvc "github.com/navidrome/navidrome/core/playlists"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
)

const (
	importFinishTimeout = 45 * time.Minute
	importPollInterval  = 3 * time.Second
	importMatchTimeout  = 5 * time.Minute
	importMatchInterval = 5 * time.Second
)

// importTrack is one resolved SomeDL queue item we try to place in the playlist.
type importTrack struct {
	somedlID string
	card     card
}

// dlImport resolves a playlist/URL via SomeDL (which enqueues its tracks) and,
// once they've downloaded and scanned in, creates a Navidrome playlist from the
// ones that matched. The wait+create runs in the background (A2): it survives
// the tab closing but not a server restart.
func dlImport(sc *somedlClient, lib libraryIndex, pl playlistsvc.Playlists, tracker *dlTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL  string `json:"url"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			dlError(w, r, err)
			return
		}
		if req.URL == "" {
			dlError(w, r, fmt.Errorf("url required"))
			return
		}
		resp, err := sc.enqueue(r.Context(), []string{req.URL})
		if err != nil {
			dlError(w, r, err)
			return
		}
		tracker.record(r.Context(), resp)
		tracks := parseSongList(resp)
		if len(tracks) == 0 {
			dlError(w, r, fmt.Errorf("SomeDL resolved no tracks from %q", req.URL))
			return
		}
		name := req.Name
		if name == "" {
			name = "Import " + time.Now().Format("2006-01-02 15:04")
		}

		user, _ := request.UserFrom(r.Context())
		go runPlaylistImport(user, tracker, lib, pl, name, tracks)

		writeJSON(w, map[string]any{"ok": true, "enqueued": len(tracks), "playlist": name})
	}
}

// parseSongList extracts resolved tracks from SomeDL's /add response
// ({song_list:[...]}). Item keys are SomeDL's (song_title, artist_all_names,
// song_id, somedl_id); read defensively so non-playlist inputs degrade rather
// than break.
func parseSongList(raw json.RawMessage) []importTrack {
	var resp struct {
		SongList []map[string]any `json:"song_list"`
	}
	_ = json.Unmarshal(raw, &resp)
	var tracks []importTrack
	for _, it := range resp.SongList {
		id := firstStr(it, "somedl_id")
		if id == "" {
			continue
		}
		tracks = append(tracks, importTrack{
			somedlID: id,
			card: card{
				Kind:    "track",
				ID:      firstStr(it, "song_id", "videoId", "original_url_id"),
				Title:   firstStr(it, "song_title", "title"),
				Artists: songArtists(it),
			},
		})
	}
	return tracks
}

// runPlaylistImport waits for the enqueued tracks to download and scan in, then
// creates a playlist from those that matched. Runs detached from the request.
func runPlaylistImport(user model.User, tracker *dlTracker, lib libraryIndex, pl playlistsvc.Playlists, name string, tracks []importTrack) {
	ctx := request.WithUser(context.Background(), user)
	ctx, cancel := context.WithTimeout(ctx, importFinishTimeout+importMatchTimeout)
	defer cancel()

	pending := make([]string, 0, len(tracks))
	for _, t := range tracks {
		pending = append(pending, t.somedlID)
	}

	if !waitFinished(ctx, tracker, pending) {
		log.Warn(ctx, "navidrome-dl import timed out waiting for downloads", "playlist", name)
	}

	ids := matchTracks(ctx, lib, tracks)
	if len(ids) == 0 {
		log.Warn(ctx, "navidrome-dl import matched no library tracks", "playlist", name)
		return
	}

	id, err := pl.Create(ctx, "", name, ids)
	if err != nil {
		log.Error(ctx, "navidrome-dl import failed to create playlist", "playlist", name, err)
		return
	}
	log.Info(ctx, "navidrome-dl import created playlist", "playlist", name, "id", id,
		"matched", len(ids), "total", len(tracks))
}

// waitFinished waits until every id has settled or ctx ends. A track being
// retried is not settled, so the playlist waits for the retry instead of
// leaving the track out.
func waitFinished(ctx context.Context, tracker *dlTracker, pending []string) bool {
	deadline := time.Now().Add(importFinishTimeout)
	for {
		done := true
		for _, id := range pending {
			if !tracker.settled(id) {
				done = false
				break
			}
		}
		if done {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(importPollInterval):
		}
	}
}

// matchTracks resolves tracks to library MediaFile ids in order, retrying until
// all match or the window elapses (files may still be scanning after download).
func matchTracks(ctx context.Context, lib libraryIndex, tracks []importTrack) []string {
	matched := make([]string, len(tracks))
	remaining := len(tracks)
	deadline := time.Now().Add(importMatchTimeout)
	for {
		for i, t := range tracks {
			if matched[i] != "" {
				continue
			}
			if id, ok := lib.trackID(ctx, t.card); ok {
				matched[i] = id
				remaining--
			}
		}
		if remaining == 0 || time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return nonEmpty(matched)
		case <-time.After(importMatchInterval):
		}
	}
	return nonEmpty(matched)
}

func firstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func songArtists(m map[string]any) []string {
	if arr, ok := m["artist_all_names"].([]any); ok {
		var names []string
		for _, a := range arr {
			if s, ok := a.(string); ok && s != "" {
				names = append(names, s)
			}
		}
		if len(names) > 0 {
			return names
		}
	}
	if s := firstStr(m, "artist_name"); s != "" {
		return []string{s}
	}
	return nil
}

// nonEmpty drops unmatched (empty) ids while preserving playlist order.
func nonEmpty(ids []string) []string {
	var out []string
	for _, id := range ids {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}
