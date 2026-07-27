package nativeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakeSomedl stands in for a running `somedl web`.
type fakeSomedl struct {
	server *httptest.Server

	mu       sync.Mutex
	finished map[string]string
	added    [][]string
	nextID   int
	addFails bool
	addEmpty bool
	gate     chan struct{} // when set, /add blocks on it before responding
}

func newFakeSomedl() *fakeSomedl {
	f := &fakeSomedl{finished: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/add", f.handleAdd)
	mux.HandleFunc("/status", f.handleStatus)
	f.server = httptest.NewServer(mux)
	return f
}

func (f *fakeSomedl) handleAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InputList []string `json:"input_list"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	f.mu.Lock()
	f.added = append(f.added, req.InputList)
	gate := f.gate
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.addFails {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	songs := []map[string]any{}
	if !f.addEmpty {
		for range req.InputList {
			f.nextID++
			songs = append(songs, map[string]any{
				"somedl_id":        fmt.Sprintf("id%d", f.nextID),
				"song_id":          "vid1",
				"original_url_id":  "vid1",
				"song_title":       "Du schreibst Geschichte",
				"artist_all_names": []string{"Madsen"},
			})
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"song_list": songs})
}

// handleStatus reports finished_items the way SomeDL does, as [status, path].
func (f *fakeSomedl) handleStatus(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	finished := map[string][]any{}
	for id, status := range f.finished {
		finished[id] = []any{status, "/music/" + id + ".mp3"}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"active_items":   map[string]any{},
		"finished_items": finished,
		"items_in_queue": 0,
	})
}

func (f *fakeSomedl) finish(id, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finished[id] = status
}

func (f *fakeSomedl) addCalls() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]string{}, f.added...)
}

var _ = Describe("dlTracker", func() {
	var fake *fakeSomedl
	var tracker *dlTracker
	var clock time.Time
	ctx := context.Background()

	// enqueue puts one item through /add the way the handlers do.
	enqueue := func(item string) string {
		resp, err := tracker.sc.enqueue(ctx, []string{item})
		Expect(err).ToNot(HaveOccurred())
		tracker.record(ctx, resp)
		var r struct {
			SongList []map[string]any `json:"song_list"`
		}
		Expect(json.Unmarshal(resp, &r)).To(Succeed())
		Expect(r.SongList).To(HaveLen(1))
		return r.SongList[0]["somedl_id"].(string)
	}

	BeforeEach(func() {
		DeferCleanup(configtest.SetupConfig())
		conf.Server.DL.MaxRetries = 2
		conf.Server.DL.RetryInitialDelay = 2 * time.Minute
		conf.Server.DL.RetryMaxDelay = 30 * time.Minute
		conf.Server.DL.RetryPollInterval = 15 * time.Second
		conf.Server.DL.RetryHistoryTTL = 24 * time.Hour

		fake = newFakeSomedl()
		DeferCleanup(fake.server.Close)

		clock = time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
		tracker = newDLTracker(newSomedlClient(fake.server.URL))
		tracker.now = func() time.Time { return clock }
	})

	It("leaves a successful download alone", func() {
		id := enqueue("https://music.youtube.com/watch?v=vid1")
		fake.finish(id, "success")

		tracker.reconcile(ctx)

		retrying, failed := tracker.snapshot()
		Expect(retrying).To(BeEmpty())
		Expect(failed).To(BeEmpty())
		Expect(fake.addCalls()).To(HaveLen(1))
		Expect(tracker.settled(id)).To(BeTrue())
	})

	It("does not treat already_downloaded as a failure", func() {
		id := enqueue("https://music.youtube.com/watch?v=vid1")
		fake.finish(id, "already_downloaded")

		tracker.reconcile(ctx)

		retrying, failed := tracker.snapshot()
		Expect(retrying).To(BeEmpty())
		Expect(failed).To(BeEmpty())
		Expect(fake.addCalls()).To(HaveLen(1))
	})

	It("waits out the backoff before retrying a failure", func() {
		id := enqueue("https://music.youtube.com/watch?v=vid1")
		fake.finish(id, "failed")

		tracker.reconcile(ctx)

		retrying, failed := tracker.snapshot()
		Expect(failed).To(BeEmpty())
		Expect(retrying).To(HaveLen(1))
		Expect(retrying[0].Label).To(Equal("Madsen - Du schreibst Geschichte"))
		Expect(fake.addCalls()).To(HaveLen(1), "must not re-enqueue immediately")

		By("still waiting one minute later")
		clock = clock.Add(time.Minute)
		tracker.reconcile(ctx)
		Expect(fake.addCalls()).To(HaveLen(1))

		By("retrying once the delay has elapsed")
		clock = clock.Add(5 * time.Minute)
		tracker.reconcile(ctx)
		calls := fake.addCalls()
		Expect(calls).To(HaveLen(2))
		Expect(calls[1]).To(Equal([]string{"https://music.youtube.com/watch?v=vid1"}))

		retrying, _ = tracker.snapshot()
		Expect(retrying).To(BeEmpty(), "the retry is pending on SomeDL again")
		Expect(tracker.settled(id)).To(BeFalse(), "the retry still belongs to the original item")
	})

	It("does not report an item as settled while its retry is in flight", func() {
		id := enqueue("https://music.youtube.com/watch?v=vid1")
		fake.finish(id, "failed")
		tracker.reconcile(ctx)
		clock = clock.Add(conf.Server.DL.RetryMaxDelay * 2)

		release := make(chan struct{})
		fake.mu.Lock()
		fake.gate = release
		fake.mu.Unlock()

		done := make(chan struct{})
		go func() {
			defer close(done)
			tracker.reconcile(ctx)
		}()

		Eventually(fake.addCalls).Should(HaveLen(2))
		Expect(tracker.settled(id)).To(BeFalse(), "the retry is still being handed over")

		close(release)
		Eventually(done).Should(BeClosed())
		Expect(tracker.settled(id)).To(BeFalse())
	})

	It("tracks nothing when the /add response does not decode", func() {
		tracker.record(ctx, json.RawMessage(`{"song_list": "not a list"}`))
		tracker.record(ctx, json.RawMessage(`{"song_list": [{"song_title": "no somedl_id"}]}`))

		tracker.reconcile(ctx)

		retrying, failed := tracker.snapshot()
		Expect(retrying).To(BeEmpty())
		Expect(failed).To(BeEmpty())
		Expect(tracker.items).To(BeEmpty())
	})

	It("leaves an item pending when its status does not decode", func() {
		id := enqueue("https://music.youtube.com/watch?v=vid1")
		fake.finish(id, "")

		tracker.reconcile(ctx)

		retrying, failed := tracker.snapshot()
		Expect(retrying).To(BeEmpty())
		Expect(failed).To(BeEmpty())
		Expect(tracker.settled(id)).To(BeFalse(), "an unreadable status must not read as finished")
	})

	It("gives up after the configured number of retries", func() {
		id := enqueue("https://music.youtube.com/watch?v=vid1")
		current := id

		for attempt := 0; attempt <= conf.Server.DL.MaxRetries; attempt++ {
			fake.finish(current, "failed")
			tracker.reconcile(ctx)
			clock = clock.Add(conf.Server.DL.RetryMaxDelay * 2)
			tracker.reconcile(ctx)
			if calls := fake.addCalls(); len(calls) > attempt+1 {
				current = fmt.Sprintf("id%d", attempt+2)
			}
		}

		Expect(fake.addCalls()).To(HaveLen(1+conf.Server.DL.MaxRetries),
			"one first attempt plus MaxRetries retries")
		retrying, failed := tracker.snapshot()
		Expect(retrying).To(BeEmpty())
		Expect(failed).To(HaveLen(1))
		Expect(failed[0].Retries).To(Equal(conf.Server.DL.MaxRetries))
		Expect(failed[0].Label).To(Equal("Madsen - Du schreibst Geschichte"))
		Expect(tracker.settled(id)).To(BeTrue())
	})

	It("counts an unreachable SomeDL against the retry budget", func() {
		id := enqueue("https://music.youtube.com/watch?v=vid1")
		fake.finish(id, "failed")
		tracker.reconcile(ctx)

		fake.mu.Lock()
		fake.addFails = true
		fake.mu.Unlock()

		for range conf.Server.DL.MaxRetries {
			clock = clock.Add(conf.Server.DL.RetryMaxDelay * 2)
			tracker.reconcile(ctx)
		}

		retrying, failed := tracker.snapshot()
		Expect(retrying).To(BeEmpty())
		Expect(failed).To(HaveLen(1))
	})

	It("gives up when SomeDL resolves nothing from the retry", func() {
		id := enqueue("https://music.youtube.com/watch?v=vid1")
		fake.finish(id, "failed")
		tracker.reconcile(ctx)

		fake.mu.Lock()
		fake.addEmpty = true
		fake.mu.Unlock()

		for range conf.Server.DL.MaxRetries {
			clock = clock.Add(conf.Server.DL.RetryMaxDelay * 2)
			tracker.reconcile(ctx)
		}

		_, failed := tracker.snapshot()
		Expect(failed).To(HaveLen(1))
	})

	It("restarts a given-up download on request", func() {
		id := enqueue("https://music.youtube.com/watch?v=vid1")
		current := id
		for attempt := 0; attempt <= conf.Server.DL.MaxRetries; attempt++ {
			fake.finish(current, "failed")
			tracker.reconcile(ctx)
			clock = clock.Add(conf.Server.DL.RetryMaxDelay * 2)
			tracker.reconcile(ctx)
			current = fmt.Sprintf("id%d", attempt+2)
		}
		_, failed := tracker.snapshot()
		Expect(failed).To(HaveLen(1))

		Expect(tracker.retryNow(nil)).To(Equal(1))
		before := len(fake.addCalls())
		tracker.reconcile(ctx)

		Expect(fake.addCalls()).To(HaveLen(before + 1))
		retrying, failed := tracker.snapshot()
		Expect(retrying).To(BeEmpty())
		Expect(failed).To(BeEmpty())
	})

	It("leaves a download that is still waiting out of a bulk retry", func() {
		id := enqueue("https://music.youtube.com/watch?v=vid1")
		fake.finish(id, "failed")
		tracker.reconcile(ctx)

		Expect(tracker.retryNow(nil)).To(BeZero(), "nothing has been given up on yet")
		Expect(tracker.retryNow([]string{id})).To(Equal(1), "but naming it pulls it forward")

		before := len(fake.addCalls())
		tracker.reconcile(ctx)
		Expect(fake.addCalls()).To(HaveLen(before + 1))
	})

	It("forgets terminal items once they age out", func() {
		id := enqueue("https://music.youtube.com/watch?v=vid1")
		fake.finish(id, "success")
		tracker.reconcile(ctx)
		Expect(tracker.label(id)).ToNot(BeEmpty())

		clock = clock.Add(conf.Server.DL.RetryHistoryTTL + time.Hour)
		tracker.reconcile(ctx)
		Expect(tracker.label(id)).To(BeEmpty())
	})

	Describe("real SomeDL payloads", func() {
		// Keys and values captured from a live SomeDL instance.
		const addResponse = `{"song_list": [{
			"somedl_id": "17879339764056952000014",
			"song_id": "C04vYQ3ESEw",
			"original_url_id": "C04vYQ3ESEw",
			"song_title": "Du schreibst Geschichte",
			"artist_all_names": ["Madsen"],
			"artist_name": "Madsen",
			"album_name": "Wo es beginnt",
			"video_type": "MUSIC_VIDEO_TYPE_ATV",
			"label": {"id": "17879339764056952000014", "text": "631/0 Madsen - Du schreibst Geschichte"}
		}]}`

		const statusResponse = `{
			"active_items": {},
			"finished_items": {
				"17879319733097938000003": ["success", "/var/lib/navidrome/music/Madsen/Goodbye Logik/Du schreibst Geschichte - Madsen.mp3"],
				"17879319874472392000003": ["already_downloaded", "/var/lib/navidrome/music/Queen/A Kind Of Magic (Deluxe Edition)/One Vision - Queen.mp3"],
				"17879339764056952000014": ["failed", null]
			},
			"items_in_queue": 0
		}`

		It("sends the item list under the key /add reads", func() {
			var body string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				body = string(b)
				_, _ = w.Write([]byte(`{"song_list":[]}`))
			}))
			DeferCleanup(srv.Close)

			_, err := newSomedlClient(srv.URL).enqueue(ctx, []string{"https://music.youtube.com/watch?v=vid1"})
			Expect(err).ToNot(HaveOccurred())
			Expect(body).To(MatchJSON(`{"input_list":["https://music.youtube.com/watch?v=vid1"]}`))
		})

		It("retries a failed track by its video id", func() {
			tracker.record(ctx, json.RawMessage(addResponse))
			Expect(tracker.label("17879339764056952000014")).
				To(Equal("Madsen - Du schreibst Geschichte"))

			fake.finish("17879339764056952000014", "failed")
			tracker.reconcile(ctx)
			clock = clock.Add(conf.Server.DL.RetryMaxDelay * 2)
			tracker.reconcile(ctx)

			calls := fake.addCalls()
			Expect(calls).To(HaveLen(1), "record() alone must not call /add")
			Expect(calls[0]).To(Equal([]string{"https://music.youtube.com/watch?v=C04vYQ3ESEw"}))
		})

		It("reports what an active track is working through", func() {
			// active_items shape from SomeDL's own console.active_items_test.
			const active = `{
				"active_items": {
					"17771447553066918000001": {
						"text": "1/2 The Warning - MORE",
						"data": {
							"musicbrainz": {"status": "success", "message": "Nothing"},
							"wait_queue": {"status": "hide", "message": "Nothing"},
							"downloading": {"status": "active", "message": "yt-dlp: Preparing download"}
						}
					},
					"17771447553066918000002": {
						"text": "2/2 Madsen - Lass die Musik an",
						"data": {
							"album": {"status": "success", "message": "Nothing"},
							"get_lyrics": {"status": "active", "message": "Nothing"}
						}
					}
				},
				"finished_items": {},
				"items_in_queue": 3
			}`

			out := normalizeStatus(json.RawMessage(active), tracker)
			Expect(out["queued"]).To(Equal(3))

			items := out["active"].([]statusItem)
			Expect(items).To(HaveLen(2))
			Expect(items[0].Label).To(Equal("1/2 The Warning - MORE"))
			Expect(items[0].Stage).To(Equal("downloading"), "the furthest along active stage")
			Expect(items[0].Detail).To(Equal("yt-dlp: Preparing download"))
			Expect(items[1].Stage).To(Equal("get_lyrics"))
			Expect(items[1].Detail).To(BeEmpty(), `"Nothing" is SomeDL's placeholder, not a detail`)
		})

		It("buckets a real /status response", func() {
			tracker.record(ctx, json.RawMessage(addResponse))
			fake.finish("17879319733097938000003", "success")
			fake.finish("17879319874472392000003", "already_downloaded")
			fake.finish("17879339764056952000014", "failed")
			tracker.reconcile(ctx)

			out := normalizeStatus(json.RawMessage(statusResponse), tracker)
			Expect(out["queued"]).To(Equal(0))
			Expect(out["retrying"]).To(HaveLen(1))
			Expect(out["failed"]).To(BeEmpty())

			statuses := map[string]string{}
			for _, it := range out["finished"].([]statusItem) {
				statuses[it.ID] = it.Status
			}
			Expect(statuses).To(Equal(map[string]string{
				"17879319733097938000003": "success",
				"17879319874472392000003": "already_downloaded",
				"17879339764056952000014": "failed",
			}))

			retrying := out["retrying"].([]dlRetryItem)
			Expect(retrying[0].Label).To(Equal("Madsen - Du schreibst Geschichte"))
			Expect(retrying[0].MaxRetries).To(Equal(conf.Server.DL.MaxRetries))
			Expect(retrying[0].NextTry).ToNot(BeEmpty())
		})
	})

	Describe("backoff", func() {
		It("grows exponentially and stops at RetryMaxDelay", func() {
			within := func(d, target time.Duration) {
				GinkgoHelper()
				Expect(d).To(BeNumerically(">=", target-target/4))
				Expect(d).To(BeNumerically("<=", target+target/4))
			}
			within(tracker.backoff(0), 2*time.Minute)
			within(tracker.backoff(1), 4*time.Minute)
			within(tracker.backoff(2), 8*time.Minute)
			within(tracker.backoff(3), 16*time.Minute)
			within(tracker.backoff(4), 30*time.Minute)
			within(tracker.backoff(50), 30*time.Minute)
		})

		It("jitters so a batch that failed together does not return in lockstep", func() {
			seen := map[time.Duration]bool{}
			for range 50 {
				seen[tracker.backoff(2)] = true
			}
			Expect(len(seen)).To(BeNumerically(">", 1))
		})
	})
})
