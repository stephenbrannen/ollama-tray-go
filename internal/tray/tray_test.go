package tray

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stephenbrannen/ollama-tray-go/internal/menu"
	"github.com/stephenbrannen/ollama-tray-go/internal/ollama"
	"github.com/stephenbrannen/ollama-tray-go/internal/status"
)

// hangTimeout only turns a deadlocked test into a failure; a passing test
// never waits for it.
const hangTimeout = 5 * time.Second

// foreverKeepAlive is how far out the server reports a model kept loaded.
const foreverKeepAlive = 290 * 365 * 24 * time.Hour

func loaded(names ...string) []ollama.LoadedModel {
	out := make([]ollama.LoadedModel, 0, len(names))
	for _, n := range names {
		out = append(out, ollama.LoadedModel{Name: n, SizeVRAM: 4_500_000_000})
	}
	return out
}

type fakeUI struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeUI) record(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}

// take returns the calls made since the last take.
func (f *fakeUI) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := f.calls
	f.calls = nil
	return calls
}

func (f *fakeUI) SetIcon(s status.Status)           { f.record("icon:%s", s) }
func (f *fakeUI) SetTooltip(text string)            { f.record("tooltip:%s", text) }
func (f *fakeUI) HideModel(slot int)                { f.record("hide:%d", slot) }
func (f *fakeUI) SetStopAllVisible(show bool)       { f.record("stopall:%t", show) }
func (f *fakeUI) ShowModel(slot int, t string)      { f.record("show:%d:%s", slot, t) }
func (f *fakeUI) ShowKeepLoaded(slot int, t string) { f.record("showkeep:%d:%s", slot, t) }
func (f *fakeUI) HideKeepLoaded(slot int)           { f.record("hidekeep:%d", slot) }
func (f *fakeUI) SetKeepLoadedVisible(show bool)    { f.record("keepmenu:%t", show) }

type fakeServer struct {
	mu     sync.Mutex
	models []ollama.LoadedModel
	err    error
	polls  int
	kept   []string
	// onPoll, onUnload and onKeep replace the default behavior when set.
	onPoll   func(ctx context.Context)
	onUnload func(ctx context.Context, name string) error
	onKeep   func(ctx context.Context, name string) error
}

func (f *fakeServer) set(models []ollama.LoadedModel, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.models, f.err = models, err
}

func (f *fakeServer) pollCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.polls
}

func (f *fakeServer) LoadedModels(ctx context.Context) ([]ollama.LoadedModel, error) {
	f.mu.Lock()
	f.polls++
	models, err, onPoll := f.models, f.err, f.onPoll
	f.mu.Unlock()
	if onPoll != nil {
		onPoll(ctx)
	}
	return models, err
}

func (f *fakeServer) Unload(ctx context.Context, name string) error {
	if f.onUnload == nil {
		return nil
	}
	return f.onUnload(ctx, name)
}

// KeepLoaded does what the server does: the model's expiry moves out to the
// far future, which the next LoadedModels reports.
func (f *fakeServer) KeepLoaded(ctx context.Context, name string) error {
	f.mu.Lock()
	f.kept = append(f.kept, name)
	models := slices.Clone(f.models)
	for i := range models {
		if models[i].Name == name {
			models[i].ExpiresAt = time.Now().Add(foreverKeepAlive)
		}
	}
	f.models = models
	onKeep := f.onKeep
	f.mu.Unlock()
	if onKeep == nil {
		return nil
	}
	return onKeep(ctx, name)
}

func newTestApp(srv *fakeServer) (*app, *fakeUI) {
	a := newApp(Config{Client: srv, Host: "host", Interval: time.Minute})
	fu := &fakeUI{}
	a.ui = fu
	return a, fu
}

func queued(a *app) []action {
	var out []action
	for {
		select {
		case act := <-a.actions:
			out = append(out, act)
		default:
			return out
		}
	}
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(hangTimeout):
		require.FailNow(t, "timed out waiting on a channel")
		panic("unreachable")
	}
}

func TestPollOnceSendsOnlyChanges(t *testing.T) {
	t.Parallel()

	hidden := func(from int) []string {
		var out []string
		for i := from; i < menu.PoolSize; i++ {
			out = append(out, fmt.Sprintf("hide:%d", i), fmt.Sprintf("hidekeep:%d", i))
		}
		return out
	}
	steps := []struct {
		name   string
		models []ollama.LoadedModel
		err    error
		want   []string
	}{
		{
			name:   "first poll applies everything",
			models: loaded("a"),
			want: append(append([]string{
				"icon:loaded", "tooltip:host\na (4.5 GB VRAM)", "show:0:Stop a", "showkeep:0:a",
			}, hidden(1)...), "keepmenu:true", "stopall:false"),
		},
		{name: "same state sends nothing", models: loaded("a"), want: nil},
		{
			name:   "second model adds one item and stop all",
			models: loaded("a", "b"),
			want:   []string{"tooltip:host\na (4.5 GB VRAM)\nb (4.5 GB VRAM)", "show:1:Stop b", "showkeep:1:b", "stopall:true"},
		},
		{
			name:   "renamed model updates only its slot",
			models: loaded("c", "b"),
			want:   []string{"tooltip:host\nc (4.5 GB VRAM)\nb (4.5 GB VRAM)", "show:0:Stop c", "showkeep:0:c"},
		},
		{
			name: "server error hides everything",
			err:  errors.New("down"),
			want: []string{
				"icon:unreachable", "tooltip:host\nunreachable", "hide:0", "hidekeep:0", "hide:1", "hidekeep:1", "keepmenu:false", "stopall:false",
			},
		},
		{name: "same error sends nothing", err: errors.New("still down"), want: nil},
	}

	srv := &fakeServer{}
	a, fu := newTestApp(srv)
	for _, step := range steps {
		srv.set(step.models, step.err)
		a.pollOnce(t.Context())
		assert.Equal(t, step.want, fu.take(), step.name)
	}
}

func TestStopModelQueuesTheNameFromTheLastPoll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		slot  int
		model []ollama.LoadedModel
		want  []action
	}{
		{"visible slot", 1, loaded("a", "b"), []action{{actionStop, []string{"b"}}}},
		{"hidden slot", 3, loaded("a", "b"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := &fakeServer{}
			a, _ := newTestApp(srv)
			srv.set(tt.model, nil)
			a.pollOnce(t.Context())
			// The server changes after the poll; the click still stops what the label showed.
			srv.set(loaded("x", "y"), nil)

			a.stopModel(tt.slot)

			assert.Equal(t, tt.want, queued(a))
		})
	}
}

func TestKeepModelQueuesOnlyForAModelThatCanBeKept(t *testing.T) {
	t.Parallel()

	kept := loaded("a", "b")
	kept[1].ExpiresAt = time.Now().Add(foreverKeepAlive)
	tests := []struct {
		name string
		slot int
		want []action
	}{
		{"model with a timer", 0, []action{{actionKeepLoaded, []string{"a"}}}},
		{"model already kept", 1, nil},
		{"hidden slot", 3, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := &fakeServer{}
			a, _ := newTestApp(srv)
			srv.set(kept, nil)
			a.pollOnce(t.Context())

			a.keepModel(tt.slot)

			assert.Equal(t, tt.want, queued(a))
		})
	}
}

func TestKeepLoadedClickChangesTheNextPoll(t *testing.T) {
	t.Parallel()
	srv := &fakeServer{}
	a, fu := newTestApp(srv)
	srv.set(loaded("a"), nil)
	a.pollOnce(t.Context())
	fu.take()
	go a.runActions(t.Context())

	a.keepModel(0)
	receive(t, a.refresh)
	a.pollOnce(t.Context())

	assert.Equal(t, []string{"a"}, srv.kept)
	assert.Equal(t, []string{"tooltip:host\na (4.5 GB VRAM, kept loaded)", "hidekeep:0", "keepmenu:false"}, fu.take())
}

func TestKeepLoadedFailureShowsInTheTooltip(t *testing.T) {
	t.Parallel()
	srv := &fakeServer{onKeep: func(context.Context, string) error { return errors.New("boom") }}
	a, fu := newTestApp(srv)
	srv.set(loaded("a"), nil)

	a.keepLoaded(t.Context(), "a")
	a.pollOnce(t.Context())

	assert.Contains(t, fu.take(), "tooltip:host\na (4.5 GB VRAM, kept loaded)\nkeep loaded failed: a: boom")
}

func TestStopEveryModelQueuesAllModelsFromTheLastPoll(t *testing.T) {
	t.Parallel()
	srv := &fakeServer{}
	a, _ := newTestApp(srv)
	srv.set(loaded("a", "b", "c"), nil)
	a.pollOnce(t.Context())

	a.stopEveryModel()

	assert.Equal(t, []action{{actionStop, []string{"a", "b", "c"}}}, queued(a))
}

func TestEnqueueStopDropsInsteadOfBlockingWhenFull(t *testing.T) {
	t.Parallel()
	a, _ := newTestApp(&fakeServer{})

	for range actionQueueSize + 1 {
		a.enqueue(action{kind: actionStop, names: []string{"a"}})
	}

	assert.Len(t, queued(a), actionQueueSize)
}

func TestClicksDuringAStopAreQueuedNotLost(t *testing.T) {
	t.Parallel()
	started := make(chan string, 8)
	release := make(chan struct{})
	srv := &fakeServer{onUnload: func(ctx context.Context, name string) error {
		started <- name
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	a, _ := newTestApp(srv)
	srv.set(loaded("a", "b"), nil)
	a.pollOnce(t.Context())
	go a.runActions(t.Context())

	a.stopModel(0)
	assert.Equal(t, "a", receive(t, started))
	// The worker is now stuck in the unload of "a"; these clicks must return.
	a.stopModel(1)
	a.stopEveryModel()
	close(release)

	got := []string{receive(t, started), receive(t, started), receive(t, started)}
	assert.ElementsMatch(t, []string{"b", "a", "b"}, got)
}

func TestUnloadRunsConcurrentlyUpToTheLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		count    int
		wantPeak int
	}{
		{"all at once under the limit", 4, 4},
		{"capped at the limit", maxConcurrentUnloads + 5, maxConcurrentUnloads},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var (
				mu       sync.Mutex
				inflight int
				peak     int
				arrived  int
				allIn    = make(chan struct{})
			)
			srv := &fakeServer{onUnload: func(ctx context.Context, _ string) error {
				mu.Lock()
				inflight++
				peak = max(peak, inflight)
				arrived++
				if arrived == tt.wantPeak {
					close(allIn)
				}
				mu.Unlock()
				defer func() {
					mu.Lock()
					inflight--
					mu.Unlock()
				}()
				// A serial unload never lets wantPeak calls arrive, so ctx ends the wait.
				select {
				case <-allIn:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}}
			a, _ := newTestApp(srv)
			names := make([]string, tt.count)
			for i := range names {
				names[i] = fmt.Sprintf("m%d", i)
			}
			ctx, cancel := context.WithTimeout(t.Context(), hangTimeout)
			defer cancel()

			a.unload(ctx, names...)

			assert.Equal(t, tt.wantPeak, peak)
			assert.NoError(t, a.failure.Err)
		})
	}
}

func TestUnloadReportsEveryFailureAndClearsItOnSuccess(t *testing.T) {
	t.Parallel()
	srv := &fakeServer{onUnload: func(_ context.Context, name string) error {
		if name == "b" {
			return nil
		}
		return errors.New("boom")
	}}
	a, fu := newTestApp(srv)

	a.unload(t.Context(), "a", "b", "c")
	a.pollOnce(t.Context())

	require.EqualError(t, a.failure.Err, "a: boom\nc: boom")
	assert.Contains(t, fu.take(), "tooltip:host\nno model loaded\nstop failed: a: boom c: boom")
	assert.Len(t, a.refresh, 1)

	a.unload(t.Context(), "b")

	assert.NoError(t, a.failure.Err)
}

func TestStopFailureExpiresAfterTheTimeToLive(t *testing.T) {
	t.Parallel()
	srv := &fakeServer{onUnload: func(context.Context, string) error { return errors.New("boom") }}
	a, fu := newTestApp(srv)
	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	a.now = func() time.Time { return clock }

	a.unload(t.Context(), "a")
	clock = clock.Add(menu.ActionFailureTTL - time.Second)
	a.pollOnce(t.Context())
	assert.Contains(t, fu.take(), "tooltip:host\nno model loaded\nstop failed: a: boom")

	clock = clock.Add(2 * time.Second)
	a.pollOnce(t.Context())
	assert.Equal(t, []string{"tooltip:host\nno model loaded"}, fu.take())
}

func TestStopAttemptClearsTheLastFailureBeforeItFinishes(t *testing.T) {
	t.Parallel()
	srv := &fakeServer{}
	a, _ := newTestApp(srv)
	a.failure = menu.ActionFailure{Action: "stop", Err: errors.New("old"), At: a.now()}
	var seen error
	srv.onUnload = func(context.Context, string) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		seen = a.failure.Err
		return errors.New("new")
	}

	a.unload(t.Context(), "a")

	require.NoError(t, seen)
	assert.EqualError(t, a.failure.Err, "a: new")
}

func TestWatchRunsOnClickPerClickUntilContextEnds(t *testing.T) {
	t.Parallel()
	a, _ := newTestApp(&fakeServer{})
	ctx, cancel := context.WithCancel(t.Context())
	clicks := make(chan struct{})
	handled := make(chan struct{}, 2)
	done := make(chan struct{})
	go func() {
		a.watch(ctx, clicks, func() { handled <- struct{}{} })
		close(done)
	}()

	clicks <- struct{}{}
	clicks <- struct{}{}
	receive(t, handled)
	receive(t, handled)
	cancel()

	receive(t, done)
}

func TestPollLoopPollsOnStartTickRefreshAndStopsOnCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := &fakeServer{}
		a, _ := newTestApp(srv)
		a.cfg.Interval = time.Minute
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			a.poll(ctx)
			close(done)
		}()

		synctest.Wait()
		assert.Equal(t, 1, srv.pollCount(), "first poll")

		time.Sleep(time.Minute)
		synctest.Wait()
		assert.Equal(t, 2, srv.pollCount(), "after one interval")

		a.requestRefresh()
		synctest.Wait()
		assert.Equal(t, 3, srv.pollCount(), "after a refresh request")

		cancel()
		synctest.Wait()
		select {
		case <-done:
		default:
			assert.Fail(t, "poll did not return after cancel")
		}
	})
}

func TestPollNeverHasTwoRequestsInFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var (
			mu       sync.Mutex
			inflight int
			peak     int
		)
		release := make(chan struct{})
		srv := &fakeServer{onPoll: func(ctx context.Context) {
			mu.Lock()
			inflight++
			peak = max(peak, inflight)
			mu.Unlock()
			select {
			case <-release:
			case <-ctx.Done():
			}
			mu.Lock()
			inflight--
			mu.Unlock()
		}}
		a, _ := newTestApp(srv)
		a.cfg.Interval = time.Second
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			a.poll(ctx)
			close(done)
		}()

		// Ten ticks and a refresh arrive while the first poll hangs.
		synctest.Wait()
		time.Sleep(10 * time.Second)
		a.requestRefresh()
		synctest.Wait()
		assert.Equal(t, 1, srv.pollCount(), "polls started while the first hangs")

		close(release)
		cancel()
		<-done
		assert.Equal(t, 1, peak)
	})
}
