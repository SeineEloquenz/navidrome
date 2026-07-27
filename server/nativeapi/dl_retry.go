package nativeapi

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/utils/random"
)

const somedlFailed = "failed"

const maxTerminalItems = 500

type dlState string

const (
	dlPending dlState = "pending" // handed to SomeDL, no terminal status yet
	dlWaiting dlState = "waiting" // failed, waiting out the backoff
	dlSending dlState = "sending" // backoff elapsed, /add in flight
	dlFailed  dlState = "failed"  // out of attempts
	dlDone    dlState = "done"    // finished; kept only so the UI can label it
)

// dlItem is one track handed to SomeDL, tracked until it finishes or runs out
// of attempts.
type dlItem struct {
	id      string // current somedl_id; SomeDL assigns a new one per attempt
	origin  string // somedl_id of the first attempt, so callers can follow retries
	item    string // what to send back to /add to redo this track
	label   string
	state   dlState
	retries int
	nextTry time.Time
	updated time.Time
}

// dlTracker remembers what was handed to SomeDL and re-enqueues what fails.
// SomeDL reports failures without a reason, so a transient MusicBrainz outage
// is indistinguishable from a track that does not exist and every failure gets
// the same bounded number of retries.
type dlTracker struct {
	sc   *somedlClient
	kick chan struct{}

	mu    sync.Mutex
	items map[string]*dlItem
	now   func() time.Time
}

func newDLTracker(sc *somedlClient) *dlTracker {
	return &dlTracker{
		sc:    sc,
		kick:  make(chan struct{}, 1),
		items: map[string]*dlItem{},
		now:   time.Now,
	}
}

// record registers the tracks from an /add response, keeping their labels since
// SomeDL drops an item's name once it finishes.
func (t *dlTracker) record(ctx context.Context, raw json.RawMessage) {
	t.register(ctx, raw, 0, "", "")
}

// register stores the tracks of an /add response. An empty origin means these
// are first attempts and are their own origin. A track SomeDL queued but we
// could not read is warned about, since it downloads with no retry and no label.
func (t *dlTracker) register(ctx context.Context, raw json.RawMessage, retries int, fallbackLabel, origin string) int {
	var resp struct {
		SongList []map[string]any `json:"song_list"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		log.Warn(ctx, "navidrome-dl cannot read SomeDL's /add response, those downloads are untracked", err)
		return 0
	}

	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	added := 0
	for _, it := range resp.SongList {
		id := firstStr(it, "somedl_id")
		if id == "" {
			continue
		}
		label := songLabel(it)
		if label == "" {
			label = fallbackLabel
		}
		root := origin
		if root == "" {
			root = id
		}
		t.items[id] = &dlItem{
			id: id, origin: root, item: retryItem(it), label: label,
			state: dlPending, retries: retries, updated: now,
		}
		added++
	}
	if added < len(resp.SongList) {
		log.Warn(ctx, "navidrome-dl could not track every download SomeDL queued",
			"tracked", added, "queued", len(resp.SongList))
	}
	return added
}

// retryItem is what goes back to /add to redo a track, preferring the video id
// SomeDL resolved over the free-text query it started from.
func retryItem(it map[string]any) string {
	if id := firstStr(it, "original_url_id", "song_id"); id != "" {
		return videoURL(id)
	}
	return firstStr(it, "text_query")
}

func (t *dlTracker) label(id string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if it, ok := t.items[id]; ok {
		return it.label
	}
	return ""
}

// settled reports whether the track first enqueued as id has stopped moving,
// following it through the new ids its retries get. Unknown ids count as settled.
func (t *dlTracker) settled(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, it := range t.items {
		if it.id != id && it.origin != id {
			continue
		}
		if it.state != dlDone && it.state != dlFailed {
			return false
		}
	}
	return true
}

// applyStatus folds SomeDL's finished_items into tracked state, scheduling a
// retry for each newly failed track that still has attempts left. An item whose
// status did not decode stays pending rather than counting as finished.
func (t *dlTracker) applyStatus(finished map[string]finishedStatus) {
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, status := range finished {
		it, ok := t.items[id]
		if !ok || it.state != dlPending || status == "" {
			continue
		}
		it.updated = now
		switch {
		case status != somedlFailed:
			it.state = dlDone
		case it.item == "" || it.retries >= conf.Server.DL.MaxRetries:
			it.state = dlFailed
			log.Warn("navidrome-dl giving up on download", "track", it.label, "id", id,
				"attempts", it.retries+1)
		default:
			it.state = dlWaiting
			it.nextTry = now.Add(t.backoff(it.retries))
		}
	}
}

// dueForRetry claims the items whose backoff has elapsed. Leaving them in the
// map as dlSending is what keeps settled reporting them as unfinished, and
// stops a second tick from enqueueing them again, while the /add is in flight.
func (t *dlTracker) dueForRetry() []dlItem {
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	var due []dlItem
	for _, it := range t.items {
		if it.state == dlWaiting && !it.nextTry.After(now) {
			it.state = dlSending
			due = append(due, *it)
		}
	}
	sort.Slice(due, func(i, j int) bool { return due[i].nextTry.Before(due[j].nextTry) })
	return due
}

// reenqueue sends one item back to SomeDL. A failed or unresolvable /add
// consumes an attempt, so an unreachable SomeDL cannot cycle an item forever.
func (t *dlTracker) reenqueue(ctx context.Context, it dlItem) {
	retries := it.retries + 1
	resp, err := t.sc.enqueue(ctx, []string{it.item})
	if err == nil {
		// Register before dropping the old id, else the item is briefly absent
		// and settled reports it finished.
		if t.register(ctx, resp, retries, it.label, it.origin) > 0 {
			t.forget(it.id)
			log.Info(ctx, "navidrome-dl retrying failed download", "track", it.label,
				"attempt", retries+1, "of", conf.Server.DL.MaxRetries+1)
			return
		}
		err = errors.New("SomeDL resolved no tracks from the retry")
	}
	t.reschedule(it, retries, err)
}

func (t *dlTracker) forget(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.items, id)
}

func (t *dlTracker) reschedule(it dlItem, retries int, err error) {
	now := t.now()
	it.retries = retries
	it.updated = now
	if retries >= conf.Server.DL.MaxRetries {
		it.state = dlFailed
		log.Warn("navidrome-dl giving up on download", "track", it.label, "id", it.id,
			"attempts", retries+1, err)
	} else {
		it.state = dlWaiting
		it.nextTry = now.Add(t.backoff(retries))
		log.Warn("navidrome-dl could not re-enqueue download", "track", it.label,
			"retryAt", it.nextTry, err)
	}
	t.mu.Lock()
	t.items[it.id] = &it
	t.mu.Unlock()
}

// backoff doubles from RetryInitialDelay up to RetryMaxDelay, jittered by a
// quarter so a batch that failed together does not come back in lockstep.
func (t *dlTracker) backoff(retries int) time.Duration {
	d := conf.Server.DL.RetryInitialDelay
	maxDelay := conf.Server.DL.RetryMaxDelay
	if d <= 0 {
		return 0
	}
	for i := 0; i < retries && d < maxDelay; i++ {
		d *= 2
	}
	if maxDelay > 0 && d > maxDelay {
		d = maxDelay
	}
	jitter := int64(d / 4)
	if jitter <= 0 {
		return d
	}
	return d - time.Duration(jitter) + time.Duration(random.Int64N(2*jitter+1))
}

// retryNow restarts items with a fresh attempt budget. Named ids may also be
// pulled out of their backoff. An empty ids list only restarts what was given
// up on, so a bulk retry doesn't reset the budget of downloads still in play.
func (t *dlTracker) retryNow(ids []string) int {
	now := t.now()
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}

	restarted := 0
	t.mu.Lock()
	for id, it := range t.items {
		if len(want) > 0 && !want[id] {
			continue
		}
		restartable := it.state == dlFailed || (len(want) > 0 && it.state == dlWaiting)
		if it.item == "" || !restartable {
			continue
		}
		it.state = dlWaiting
		it.retries = 0
		it.nextTry = now
		it.updated = now
		restarted++
	}
	t.mu.Unlock()

	if restarted > 0 {
		select {
		case t.kick <- struct{}{}:
		default:
		}
	}
	return restarted
}

type dlRetryItem struct {
	ID         string `json:"id"`
	Label      string `json:"label,omitempty"`
	Retries    int    `json:"retries"`
	MaxRetries int    `json:"maxRetries"`
	NextTry    string `json:"nextTry,omitempty"`
}

// snapshot lists what is waiting on a retry and what has been given up on.
func (t *dlTracker) snapshot() (retrying, failed []dlRetryItem) {
	retrying, failed = []dlRetryItem{}, []dlRetryItem{}
	maxRetries := conf.Server.DL.MaxRetries

	t.mu.Lock()
	for _, it := range t.items {
		e := dlRetryItem{ID: it.id, Label: it.label, Retries: it.retries, MaxRetries: maxRetries}
		switch it.state {
		case dlWaiting:
			e.NextTry = it.nextTry.UTC().Format(time.RFC3339)
			retrying = append(retrying, e)
		case dlSending:
			retrying = append(retrying, e)
		case dlFailed:
			failed = append(failed, e)
		}
	}
	t.mu.Unlock()

	sort.Slice(retrying, func(i, j int) bool {
		if retrying[i].NextTry != retrying[j].NextTry {
			return retrying[i].NextTry < retrying[j].NextTry
		}
		return retrying[i].ID < retrying[j].ID
	})
	sort.Slice(failed, func(i, j int) bool { return failed[i].ID < failed[j].ID })
	return retrying, failed
}

// prune drops terminal entries once they age out and caps how many are kept.
func (t *dlTracker) prune() {
	cutoff := t.now().Add(-conf.Server.DL.RetryHistoryTTL)
	t.mu.Lock()
	defer t.mu.Unlock()

	var terminal []*dlItem
	for id, it := range t.items {
		if it.state != dlDone && it.state != dlFailed {
			continue
		}
		if it.updated.Before(cutoff) {
			delete(t.items, id)
			continue
		}
		terminal = append(terminal, it)
	}
	if len(terminal) <= maxTerminalItems {
		return
	}
	sort.Slice(terminal, func(i, j int) bool { return terminal[i].updated.Before(terminal[j].updated) })
	for _, it := range terminal[:len(terminal)-maxTerminalItems] {
		delete(t.items, it.id)
	}
}

func (t *dlTracker) reconcile(ctx context.Context) {
	if raw, err := t.sc.status(ctx); err != nil {
		log.Debug(ctx, "navidrome-dl could not read SomeDL status", err)
	} else if s, err := parseStatus(raw); err != nil {
		log.Warn(ctx, "navidrome-dl cannot read SomeDL's /status, retries are stalled", err)
	} else {
		t.applyStatus(s.FinishedItems)
	}
	for _, it := range t.dueForRetry() {
		t.reenqueue(ctx, it)
	}
	t.prune()
}

// run reconciles on a ticker until ctx ends.
func (t *dlTracker) run(ctx context.Context) {
	interval := conf.Server.DL.RetryPollInterval
	if interval <= 0 {
		interval = time.Second
		log.Warn(ctx, "navidrome-dl: DL.RetryPollInterval must be positive", "using", interval)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-t.kick:
		}
		t.reconcile(ctx)
	}
}
