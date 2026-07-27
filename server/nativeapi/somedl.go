package nativeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// somedlClient talks to a running `somedl web` instance (Flask JSON API, no auth).
type somedlClient struct {
	base string
	http *http.Client
}

func newSomedlClient(baseURL string) *somedlClient {
	return &somedlClient{
		base: strings.TrimSuffix(baseURL, "/"),
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *somedlClient) post(ctx context.Context, path string, body any) (json.RawMessage, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, path)
}

func (c *somedlClient) get(ctx context.Context, path string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	return c.do(req, path)
}

func (c *somedlClient) do(req *http.Request, path string) (json.RawMessage, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("somedl %s: %w", path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("somedl %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("somedl %s: HTTP %d: %s", path, resp.StatusCode, data)
	}
	return data, nil
}

// extractResult pulls the `result` field out of a `{ok, result}` response.
func extractResult(raw json.RawMessage) json.RawMessage {
	var wrap struct {
		Result json.RawMessage `json:"result"`
	}
	_ = json.Unmarshal(raw, &wrap)
	return wrap.Result
}

// filter is a ytmusicapi filter: "songs" | "albums" | "artists".
func (c *somedlClient) search(ctx context.Context, query, filter string) (json.RawMessage, error) {
	raw, err := c.post(ctx, "/yt-search", map[string]string{"search_query": query, "filter": filter})
	if err != nil {
		return nil, err
	}
	return extractResult(raw), nil
}

func (c *somedlClient) albumByBrowseID(ctx context.Context, browseID string) (json.RawMessage, error) {
	raw, err := c.post(ctx, "/yt-get-album-browse-id", map[string]any{"album_id": browseID, "return_album_data": true})
	if err != nil {
		return nil, err
	}
	return extractResult(raw), nil
}

func (c *somedlClient) artist(ctx context.Context, artistID string) (json.RawMessage, error) {
	raw, err := c.post(ctx, "/yt-get-artist", map[string]string{"artist_id": artistID})
	if err != nil {
		return nil, err
	}
	return extractResult(raw), nil
}

// enqueue accepts URLs or free-text queries (SomeDL auto-picks a match for text).
func (c *somedlClient) enqueue(ctx context.Context, items []string) (json.RawMessage, error) {
	return c.post(ctx, "/add", map[string]any{"input_list": items})
}

func (c *somedlClient) status(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, "/status")
}

type statusItem struct {
	ID     string `json:"id"`
	Label  string `json:"label,omitempty"`
	Status string `json:"status"`
	Stage  string `json:"stage,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// stageOrder is SomeDL's pipeline (the ids it passes to console.update). A
// track keeps every stage it has touched, so the furthest along one still
// marked active is the one worth showing.
var stageOrder = []string{
	"album", "musicbrainz", "deezer", "get_lyrics", "label",
	"wait_queue", "downloading", "albumart", "disable_download",
}

// somedlNoMessage is console.update's placeholder for a stage with no detail.
const somedlNoMessage = "Nothing"

type activeItem struct {
	Text string `json:"text"`
	Data map[string]struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"data"`
}

// stage reports what the track is working through, and any message SomeDL
// attached to it. A stage we don't know outranks the known ones so a new one
// surfaces rather than hiding behind them.
func (a activeItem) stage() (name, detail string) {
	rank := -1
	for key, st := range a.Data {
		if st.Status != "active" {
			continue
		}
		r := stageRank(key)
		if r < rank || (r == rank && key <= name) {
			continue
		}
		rank, name, detail = r, key, st.Message
	}
	if detail == somedlNoMessage {
		detail = ""
	}
	return name, detail
}

func stageRank(key string) int {
	for i, s := range stageOrder {
		if s == key {
			return i
		}
	}
	return len(stageOrder)
}

// finishedStatus is one finished_items value, which SomeDL reports as a
// [status, path] pair. Decoding it as a plain string silently yields an empty
// status for every item, which reads as a success everywhere downstream.
type finishedStatus string

func (f *finishedStatus) UnmarshalJSON(data []byte) error {
	var pair []any
	if err := json.Unmarshal(data, &pair); err != nil {
		return err
	}
	if len(pair) > 0 {
		s, _ := pair[0].(string)
		*f = finishedStatus(s)
	}
	return nil
}

type somedlStatus struct {
	ActiveItems   map[string]activeItem     `json:"active_items"`
	FinishedItems map[string]finishedStatus `json:"finished_items"`
	ItemsInQueue  int                       `json:"items_in_queue"`
}

func parseStatus(raw json.RawMessage) (somedlStatus, error) {
	var s somedlStatus
	err := json.Unmarshal(raw, &s)
	return s, err
}

func songLabel(it map[string]any) string {
	title := firstStr(it, "song_title", "title")
	artists := songArtists(it)
	switch {
	case len(artists) > 0 && title != "":
		return artists[0] + " - " + title
	case title != "":
		return title
	default:
		return firstStr(it, "text_query")
	}
}

// normalizeStatus flattens SomeDL's /status (active_items keyed by id,
// finished_items id->status, items_in_queue) into id-sorted lists, plus the
// tracker's retrying and given-up items. finished_items is process-global and
// grows unbounded; the UI scopes it to the current session.
func normalizeStatus(raw json.RawMessage, t *dlTracker) map[string]any {
	s, _ := parseStatus(raw)

	active := make([]statusItem, 0, len(s.ActiveItems))
	for id, it := range s.ActiveItems {
		stage, detail := it.stage()
		active = append(active, statusItem{
			ID: id, Label: it.Text, Status: "active", Stage: stage, Detail: detail,
		})
	}
	finished := make([]statusItem, 0, len(s.FinishedItems))
	for id, st := range s.FinishedItems {
		finished = append(finished, statusItem{ID: id, Status: string(st), Label: t.label(id)})
	}
	sort.Slice(active, func(i, j int) bool { return active[i].ID < active[j].ID })
	sort.Slice(finished, func(i, j int) bool { return finished[i].ID < finished[j].ID })

	retrying, failed := t.snapshot()
	return map[string]any{
		"active": active, "finished": finished, "queued": s.ItemsInQueue,
		"retrying": retrying, "failed": failed,
	}
}

// card is a normalized result for the Discover UI.
type card struct {
	Kind        string   `json:"kind"` // "album" | "track" | "artist"
	ID          string   `json:"id"`   // browseId (album/artist) or videoId (track)
	Title       string   `json:"title"`
	Artists     []string `json:"artists,omitempty"`
	Year        string   `json:"year,omitempty"`
	Thumbnail   string   `json:"thumbnail,omitempty"`
	Duration    string   `json:"duration,omitempty"`
	TrackNumber int      `json:"trackNumber,omitempty"`
	InLibrary   bool     `json:"inLibrary,omitempty"`
}

func normalizeSearch(kind string, result json.RawMessage) []card {
	var items []map[string]any
	_ = json.Unmarshal(result, &items)
	cards := make([]card, 0, len(items))
	for _, it := range items {
		if c, ok := normalizeItem(kind, it); ok {
			cards = append(cards, c)
		}
	}
	return cards
}

// normalizeTracks numbers album tracks by ytmusicapi's trackNumber, falling
// back to 1-based position (it's null for unavailable tracks). i indexes the
// full list, so the fallback stays aligned to YT's ordering even when
// normalizeItem drops entries.
func normalizeTracks(tracks json.RawMessage) []card {
	var items []map[string]any
	_ = json.Unmarshal(tracks, &items)
	cards := make([]card, 0, len(items))
	for i, it := range items {
		if c, ok := normalizeItem("track", it); ok {
			if n, ok := optInt(it, "trackNumber"); ok {
				c.TrackNumber = n
			} else {
				c.TrackNumber = i + 1
			}
			cards = append(cards, c)
		}
	}
	return cards
}

func normalizeItem(kind string, it map[string]any) (card, bool) {
	switch kind {
	case "track":
		id, ok := reqStr(it, "videoId")
		if !ok {
			return card{}, false
		}
		return card{Kind: "track", ID: id, Title: optStr(it, "title"),
			Artists: artistNames(it), Thumbnail: bestThumbnail(it), Duration: optStr(it, "duration")}, true
	case "album":
		id, ok := reqStr(it, "browseId")
		if !ok {
			return card{}, false
		}
		return card{Kind: "album", ID: id, Title: optStr(it, "title"),
			Artists: artistNames(it), Year: optStr(it, "year"), Thumbnail: bestThumbnail(it)}, true
	case "artist":
		id, ok := reqStr(it, "browseId")
		if !ok {
			return card{}, false
		}
		title := optStr(it, "artist") // artist search results carry the name in "artist"
		if title == "" {
			title = optStr(it, "title")
		}
		return card{Kind: "artist", ID: id, Title: title, Thumbnail: bestThumbnail(it)}, true
	}
	return card{}, false
}

// missingTracks returns candidate tracks whose number isn't present locally.
func missingTracks(present map[int]bool, candidate []card) []card {
	var missing []card
	for _, c := range candidate {
		if c.TrackNumber > 0 && !present[c.TrackNumber] {
			missing = append(missing, c)
		}
	}
	return missing
}

func reqStr(m map[string]any, key string) (string, bool) {
	v, ok := m[key].(string)
	return v, ok && v != ""
}

func optStr(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// optInt reads a JSON number (unmarshalled as float64); false when absent/null.
func optInt(m map[string]any, key string) (int, bool) {
	if v, ok := m[key].(float64); ok {
		return int(v), true
	}
	return 0, false
}

func artistNames(m map[string]any) []string {
	arr, ok := m["artists"].([]any)
	if !ok {
		return nil
	}
	var names []string
	for _, a := range arr {
		if am, ok := a.(map[string]any); ok {
			if n, ok := am["name"].(string); ok {
				names = append(names, n)
			}
		}
	}
	return names
}

// ytmusicapi thumbnails run smallest -> largest; take the largest.
func bestThumbnail(m map[string]any) string {
	arr, ok := m["thumbnails"].([]any)
	if !ok || len(arr) == 0 {
		return ""
	}
	last, ok := arr[len(arr)-1].(map[string]any)
	if !ok {
		return ""
	}
	u, _ := last["url"].(string)
	return u
}
