package taskplane

import (
	"errors"
	"testing"
)

const (
	t0  = int64(1_000_000_000)
	ttl = int64(60_000_000_000) // 60s
)

func idleStatus(name string, nextDue int64) Status {
	return Status{Name: name, Phase: PhaseIdle, NextDueNanos: nextDue}
}

func TestClaimFromIdleStartsAttempt(t *testing.T) {
	next, changed, err := Claim(idleStatus("research", 0), nil, t0, "worker-a", "tok-1", ttl)
	if err != nil || !changed {
		t.Fatalf("claim should succeed: changed=%v err=%v", changed, err)
	}
	if next.Phase != PhaseRunning || next.Owner != "worker-a" || next.Token != "tok-1" {
		t.Fatalf("unexpected status: %+v", next)
	}
	if next.Attempt != 1 || next.LeaseUntilNanos != t0+ttl {
		t.Fatalf("attempt/lease wrong: %+v", next)
	}
}

func TestClaimWhileLeasedReturnsLeased(t *testing.T) {
	cur := Status{Name: "research", Phase: PhaseRunning, Owner: "worker-a", Token: "tok-1", Attempt: 1, LeaseUntilNanos: t0 + ttl}
	_, _, err := Claim(cur, nil, t0+1, "worker-b", "tok-2", ttl)
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeLeased {
		t.Fatalf("want leased, got %v", err)
	}
	if te.Owner != "worker-a" || te.LeaseUntilNanos != t0+ttl {
		t.Fatalf("leased error should carry owner/until: %+v", te)
	}
}

func TestClaimAfterLeaseExpiryStartsNewAttempt(t *testing.T) {
	cur := Status{Name: "research", Phase: PhaseRunning, Owner: "worker-a", Token: "tok-1", Attempt: 1, LeaseUntilNanos: t0}
	next, changed, err := Claim(cur, nil, t0+1, "worker-b", "tok-2", ttl)
	if err != nil || !changed {
		t.Fatalf("expired lease should be claimable: changed=%v err=%v", changed, err)
	}
	if next.Owner != "worker-b" || next.Attempt != 2 || next.Token != "tok-2" {
		t.Fatalf("unexpected takeover: %+v", next)
	}
}

func TestClaimBeforeNextDueReturnsNotDue(t *testing.T) {
	cur := idleStatus("research", t0+100)
	_, _, err := Claim(cur, nil, t0, "worker-a", "tok-1", ttl)
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeNotDue || te.NextDueNanos != t0+100 {
		t.Fatalf("want not_due with next due, got %v", err)
	}
}

func TestClaimAbandonedIsRejected(t *testing.T) {
	cur := Status{Name: "research", Phase: PhaseAbandoned, Attempt: 3}
	_, _, err := Claim(cur, nil, t0, "worker-a", "tok-1", ttl)
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeAbandoned {
		t.Fatalf("want abandoned, got %v", err)
	}
}

func TestClaimHonoursMaxAttempts(t *testing.T) {
	spec := &Spec{Name: "research", MaxAttempts: 1}
	cur := Status{Name: "research", Phase: PhaseSucceeded, Attempt: 1, NextDueNanos: 0, Token: "old"}
	_, _, err := Claim(cur, spec, t0, "worker-a", "tok-2", ttl)
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeAbandoned {
		t.Fatalf("want abandoned after max attempts, got %v", err)
	}
}

func TestCompleteSucceedsAndSchedulesCadence(t *testing.T) {
	spec := &Spec{Name: "research", CadenceNanos: 24 * 3600 * 1e9}
	cur := Status{Name: "research", Phase: PhaseRunning, Owner: "worker-a", Token: "tok-1", Attempt: 1, LeaseUntilNanos: t0 + ttl}
	next, changed, err := Complete(cur, spec, t0+1, "tok-1", Completion{
		Checkpoint: []byte(`{"cursor":42}`),
		Refs:       []string{"proposal:abc"},
		Outcome:    []byte(`{"ok":true}`),
	})
	if err != nil || !changed {
		t.Fatalf("complete should succeed: changed=%v err=%v", changed, err)
	}
	if next.Phase != PhaseSucceeded || next.LeaseUntilNanos != 0 || next.Owner != "" {
		t.Fatalf("lease should be cleared: %+v", next)
	}
	if string(next.Checkpoint) != `{"cursor":42}` || len(next.Refs) != 1 {
		t.Fatalf("checkpoint/refs not stored: %+v", next)
	}
	if next.NextDueNanos != t0+1+spec.CadenceNanos {
		t.Fatalf("next due wrong: %d", next.NextDueNanos)
	}
}

func TestCompleteTwiceWithSameTokenIsNoop(t *testing.T) {
	spec := &Spec{Name: "research", CadenceNanos: 10}
	cur := Status{Name: "research", Phase: PhaseRunning, Token: "tok-1", Attempt: 1, LeaseUntilNanos: t0 + ttl}
	done, _, err := Complete(cur, spec, t0, "tok-1", Completion{})
	if err != nil {
		t.Fatal(err)
	}
	_, changed, err := Complete(done, spec, t0+1, "tok-1", Completion{})
	if err != nil || changed {
		t.Fatalf("duplicate complete must be idempotent: changed=%v err=%v", changed, err)
	}
}

func TestCompleteStaleTokenIsRejected(t *testing.T) {
	cur := Status{Name: "research", Phase: PhaseRunning, Token: "tok-2", Attempt: 2, LeaseUntilNanos: t0 + ttl}
	_, _, err := Complete(cur, nil, t0, "tok-1", Completion{})
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeStaleAttempt {
		t.Fatalf("want stale_attempt, got %v", err)
	}
}

func TestCompleteFromIdleIsStale(t *testing.T) {
	cur := idleStatus("research", 0)
	_, _, err := Complete(cur, nil, t0, "tok-1", Completion{})
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeStaleAttempt {
		t.Fatalf("want stale_attempt, got %v", err)
	}
}

func TestFailRetryableSchedulesRetry(t *testing.T) {
	spec := &Spec{Name: "research", MaxAttempts: 3}
	cur := Status{Name: "research", Phase: PhaseRunning, Token: "tok-1", Attempt: 1, LeaseUntilNanos: t0 + ttl}
	next, changed, err := Fail(cur, spec, t0+1, "tok-1", "boom", true)
	if err != nil || !changed {
		t.Fatalf("fail should transition: changed=%v err=%v", changed, err)
	}
	if next.Phase != PhaseIdle || next.NextDueNanos != t0+1 || next.LastError != "boom" {
		t.Fatalf("retry state wrong: %+v", next)
	}
	if next.LeaseUntilNanos != 0 || next.Owner != "" {
		t.Fatalf("lease should be cleared: %+v", next)
	}
}

func TestFailExhaustedAbandons(t *testing.T) {
	spec := &Spec{Name: "research", MaxAttempts: 1}
	cur := Status{Name: "research", Phase: PhaseRunning, Token: "tok-1", Attempt: 1, LeaseUntilNanos: t0 + ttl}
	next, changed, err := Fail(cur, spec, t0, "tok-1", "boom", true)
	if err != nil || !changed {
		t.Fatalf("fail should transition: changed=%v err=%v", changed, err)
	}
	if next.Phase != PhaseAbandoned || next.NextDueNanos != 0 {
		t.Fatalf("want abandoned: %+v", next)
	}
}

func TestFailNotRetryableAbandonsImmediately(t *testing.T) {
	cur := Status{Name: "research", Phase: PhaseRunning, Token: "tok-1", Attempt: 1, LeaseUntilNanos: t0 + ttl}
	next, _, err := Fail(cur, nil, t0, "tok-1", "fatal", false)
	if err != nil {
		t.Fatal(err)
	}
	if next.Phase != PhaseAbandoned {
		t.Fatalf("want abandoned: %+v", next)
	}
}

func TestRenewExtendsLease(t *testing.T) {
	cur := Status{Name: "research", Phase: PhaseRunning, Token: "tok-1", LeaseUntilNanos: t0 + 10}
	next, changed, err := Renew(cur, t0+1, "tok-1", ttl)
	if err != nil || !changed {
		t.Fatalf("renew should extend: changed=%v err=%v", changed, err)
	}
	if next.LeaseUntilNanos != t0+1+ttl {
		t.Fatalf("lease not extended: %d", next.LeaseUntilNanos)
	}
}

func TestRenewAfterExpiryIsStale(t *testing.T) {
	cur := Status{Name: "research", Phase: PhaseRunning, Token: "tok-1", LeaseUntilNanos: t0}
	_, _, err := Renew(cur, t0+1, "tok-1", ttl)
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeStaleAttempt {
		t.Fatalf("want stale_attempt, got %v", err)
	}
}

func TestCheckpointPersistsAndIsIdempotent(t *testing.T) {
	cur := Status{Name: "research", Phase: PhaseRunning, Token: "tok-1", Attempt: 1, LeaseUntilNanos: t0 + ttl}
	next, changed, err := Checkpoint(cur, t0, "tok-1", []byte(`{"cursor":7}`), []string{"evidence:1"})
	if err != nil || !changed {
		t.Fatalf("checkpoint should persist: changed=%v err=%v", changed, err)
	}
	if string(next.Checkpoint) != `{"cursor":7}` || len(next.Refs) != 1 {
		t.Fatalf("checkpoint/refs not stored: %+v", next)
	}
	_, changed, err = Checkpoint(next, t0, "tok-1", []byte(`{"cursor":7}`), []string{"evidence:1"})
	if err != nil || changed {
		t.Fatalf("identical checkpoint must be no-op: changed=%v err=%v", changed, err)
	}
}

func TestCheckpointAfterExpiryIsStale(t *testing.T) {
	cur := Status{Name: "research", Phase: PhaseRunning, Token: "tok-1", LeaseUntilNanos: t0}
	_, _, err := Checkpoint(cur, t0+1, "tok-1", []byte("cp"), nil)
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeStaleAttempt {
		t.Fatalf("want stale_attempt, got %v", err)
	}
}

func TestCheckpointRejectsNonOwner(t *testing.T) {
	cur := Status{Name: "research", Phase: PhaseRunning, Token: "tok-1", LeaseUntilNanos: t0 + ttl}
	_, _, err := Checkpoint(cur, t0, "tok-2", []byte("cp"), nil)
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeStaleAttempt {
		t.Fatalf("want stale_attempt, got %v", err)
	}
}

func TestClaimRejectsEmptyWorkerAndTTL(t *testing.T) {
	for _, tc := range []struct {
		worker string
		ttl    int64
	}{{"", ttl}, {"worker", 0}, {"worker", -1}} {
		if _, _, err := Claim(idleStatus("research", 0), nil, t0, tc.worker, "tok", tc.ttl); err == nil {
			t.Fatalf("want invalid error for worker=%q ttl=%d", tc.worker, tc.ttl)
		}
	}
}

func TestSpecValidation(t *testing.T) {
	if err := (&Spec{}).Validate(); err == nil {
		t.Fatal("empty name must be invalid")
	}
	if err := (&Spec{Name: "x", CadenceNanos: -1}).Validate(); err == nil {
		t.Fatal("negative cadence must be invalid")
	}
	if err := (&Spec{Name: "x", CadenceNanos: 1, MaxAttempts: 2}).Validate(); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
}

func TestStatusPhaseValidation(t *testing.T) {
	for _, p := range []Phase{PhaseIdle, PhaseRunning, PhaseSucceeded, PhaseFailed, PhaseAbandoned} {
		if !p.Valid() {
			t.Fatalf("phase %q should be valid", p)
		}
	}
	if Phase("bogus").Valid() {
		t.Fatal("bogus phase must be invalid")
	}
}
