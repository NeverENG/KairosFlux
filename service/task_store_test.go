package service

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChronoBrew/KairosFlux/internal/taskplane"
)

// testClock 是可推进的假时钟，供任务租约/到期语义测试使用（不依赖真实时间，
// 避免 sleep 与 flaky）。
type testClock struct {
	nanos atomic.Int64
}

func (c *testClock) now() time.Time          { return time.Unix(0, c.nanos.Load()) }
func (c *testClock) set(nanos int64)         { c.nanos.Store(nanos) }
func (c *testClock) advance(d time.Duration) { c.nanos.Add(int64(d)) }

func newTestTaskStore(t *testing.T) (*TaskStore, *testClock) {
	t.Helper()
	kv := setupTemporalTest(t)
	clock := &testClock{}
	var tok atomic.Uint64
	store := newTaskStore(NewTemporalStore(kv), clock.now, func() (string, error) {
		return fmt.Sprintf("tok-%d", tok.Add(1)), nil
	})
	return store, clock
}

func TestTaskStore_DefineSpecThenClaim(t *testing.T) {
	store, clock := newTestTaskStore(t)
	now := int64(1_000_000_000)
	clock.set(now)

	spec := taskplane.Spec{Name: "research:daily", Kind: "research", CadenceNanos: int64(24 * time.Hour)}
	if _, err := store.DefineSpec(spec); err != nil {
		t.Fatalf("define spec: %v", err)
	}

	got, err := store.Claim("research:daily", "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if got.Phase != taskplane.PhaseRunning || got.Attempt != 1 || got.Owner != "worker-a" || got.Token != "tok-1" {
		t.Fatalf("unexpected claim status: %+v", got)
	}
	if got.LeaseUntilNanos != now+int64(time.Minute) {
		t.Fatalf("lease until wrong: %d", got.LeaseUntilNanos)
	}
}

func TestTaskStore_ClaimWhileLeasedReportsOwner(t *testing.T) {
	store, clock := newTestTaskStore(t)
	clock.set(1_000_000_000)
	if _, err := store.Claim("research:daily", "worker-a", time.Minute); err != nil {
		t.Fatal(err)
	}

	_, err := store.Claim("research:daily", "worker-b", time.Minute)
	var te *taskplane.Error
	if !errors.As(err, &te) || te.Code != taskplane.CodeLeased || te.Owner != "worker-a" {
		t.Fatalf("want leased by worker-a, got %v", err)
	}
}

func TestTaskStore_CompleteSchedulesNextCycle(t *testing.T) {
	store, clock := newTestTaskStore(t)
	now := int64(1_000_000_000)
	clock.set(now)
	cadence := int64(24 * time.Hour)
	if _, err := store.DefineSpec(taskplane.Spec{Name: "research:daily", CadenceNanos: cadence}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim("research:daily", "worker-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	done, err := store.Complete("research:daily", claimed.Token, taskplane.Completion{
		Checkpoint: []byte(`{"step":"done"}`),
		Refs:       []string{"proposal:abc"},
		Outcome:    []byte(`{"ok":true}`),
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if done.Phase != taskplane.PhaseSucceeded || done.NextDueNanos != now+cadence {
		t.Fatalf("unexpected completion: %+v", done)
	}

	// 未到 next_due 再次 claim → not_due
	if _, err := store.Claim("research:daily", "worker-b", time.Minute); err == nil {
		t.Fatal("claim before next_due must fail")
	}

	clock.advance(time.Duration(cadence))
	again, err := store.Claim("research:daily", "worker-b", time.Minute)
	if err != nil {
		t.Fatalf("claim after due: %v", err)
	}
	if again.Attempt != 2 {
		t.Fatalf("attempt should be 2, got %d", again.Attempt)
	}
}

func TestTaskStore_CrashResumeKeepsCheckpoint(t *testing.T) {
	store, clock := newTestTaskStore(t)
	clock.set(1_000_000_000)
	claimed, err := store.Claim("research:long", "worker-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Checkpoint("research:long", claimed.Token, []byte(`{"cursor":42}`), []string{"evidence:1"}); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}

	// worker-a 崩溃：租约过期后 worker-b 接管，断点仍在。
	clock.advance(2 * time.Minute)
	taken, err := store.Claim("research:long", "worker-b", time.Minute)
	if err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if taken.Attempt != 2 || string(taken.Checkpoint) != `{"cursor":42}` {
		t.Fatalf("resume lost checkpoint: %+v", taken)
	}

	// 旧 token 不能提交结果。
	if _, err := store.Complete("research:long", claimed.Token, taskplane.Completion{}); err == nil {
		t.Fatal("stale token must not complete")
	}
	if _, err := store.Complete("research:long", taken.Token, taskplane.Completion{}); err != nil {
		t.Fatalf("new owner complete: %v", err)
	}
}

func TestTaskStore_ConcurrentClaimSingleWinner(t *testing.T) {
	store, clock := newTestTaskStore(t)
	clock.set(1_000_000_000)

	const competitors = 8
	var wg sync.WaitGroup
	results := make(chan error, competitors)
	for range competitors {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Claim("research:race", "worker", time.Minute)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	winners := 0
	for err := range results {
		if err == nil {
			winners++
			continue
		}
		var te *taskplane.Error
		if !errors.As(err, &te) || te.Code != taskplane.CodeLeased {
			t.Fatalf("loser must get leased, got %v", err)
		}
	}
	if winners != 1 {
		t.Fatalf("want exactly one winner, got %d", winners)
	}
}

func TestTaskStore_StatusReportsLatest(t *testing.T) {
	store, clock := newTestTaskStore(t)
	clock.set(1)
	if _, found, err := store.Status("research:none"); err != nil || found {
		t.Fatalf("unknown task must be absent: found=%v err=%v", found, err)
	}
	if _, err := store.Claim("research:none", "worker", time.Minute); err != nil {
		t.Fatal(err)
	}
	st, found, err := store.Status("research:none")
	if err != nil || !found || st.Phase != taskplane.PhaseRunning {
		t.Fatalf("unexpected status: %+v found=%v err=%v", st, found, err)
	}
}
