package taskplane

import "fmt"

// Claim 尝试取得任务的一次执行租约。
//
// 语义：
//   - running 且租约未过期 → CodeLeased（携带当前持有者与到期时间）；
//   - abandoned → CodeAbandoned；
//   - now < next_due → CodeNotDue（携带 next_due，调用方按需重试）；
//   - 达到 max_attempts → CodeAbandoned；
//   - 其余情况产生新状态：phase=running、attempt+1、owner/token/lease 更新。
//
// checkpoint/refs/outcome 跨 attempt 保留：下一次 attempt 从上次断点继续，
// 而不是从零重来。token 由调用方生成（纯函数不引入随机性）。
func Claim(cur Status, spec *Spec, now int64, worker, token string, ttlNanos int64) (Status, bool, error) {
	if worker == "" {
		return cur, false, &Error{Code: CodeInvalid, Message: "worker is empty"}
	}
	if token == "" {
		return cur, false, &Error{Code: CodeInvalid, Message: "token is empty"}
	}
	if ttlNanos <= 0 {
		return cur, false, &Error{Code: CodeInvalid, Message: "ttl must be > 0"}
	}
	cur, err := normalize(cur, spec)
	if err != nil {
		return cur, false, err
	}

	switch cur.Phase {
	case PhaseAbandoned:
		return cur, false, &Error{Code: CodeAbandoned, Owner: cur.Owner}
	case PhaseRunning:
		if cur.LeaseUntilNanos > now {
			return cur, false, &Error{Code: CodeLeased, Owner: cur.Owner, LeaseUntilNanos: cur.LeaseUntilNanos}
		}
	}
	if now < cur.NextDueNanos {
		return cur, false, &Error{Code: CodeNotDue, NextDueNanos: cur.NextDueNanos}
	}
	if spec != nil && spec.MaxAttempts > 0 && cur.Attempt >= spec.MaxAttempts {
		return cur, false, &Error{Code: CodeAbandoned, Owner: cur.Owner, Message: "max attempts reached"}
	}

	next := cur
	next.Phase = PhaseRunning
	next.Owner = worker
	next.Token = token
	next.Attempt = cur.Attempt + 1
	next.LeaseUntilNanos = now + ttlNanos
	next.LastError = ""
	return next, true, nil
}

// Complete 把 running 中的 attempt 标记为成功。
//
// 幂等：同一 token 重复 complete 返回 changed=false；token 不匹配、或阶段
// 不是 running，返回 CodeStaleAttempt。成功时按 spec.CadenceNanos 计算
// next_due（无节奏则 0，立即再次到期）。
func Complete(cur Status, spec *Spec, now int64, token string, done Completion) (Status, bool, error) {
	cur, err := normalize(cur, spec)
	if err != nil {
		return cur, false, err
	}
	if token == "" {
		return cur, false, &Error{Code: CodeInvalid, Message: "token is empty"}
	}
	if cur.Token == token && cur.Phase == PhaseSucceeded {
		return cur, false, nil // 重复提交：幂等
	}
	if cur.Phase != PhaseRunning || cur.Token != token {
		return cur, false, &Error{Code: CodeStaleAttempt, Owner: cur.Owner, Message: "token does not own the task"}
	}

	next := cur
	next.Phase = PhaseSucceeded
	next.Owner = ""
	next.LeaseUntilNanos = 0
	next.Checkpoint = done.Checkpoint
	next.Refs = done.Refs
	next.Outcome = done.Outcome
	next.LastError = ""
	next.NextDueNanos = nextDueAfter(spec, now)
	return next, true, nil
}

// Fail 把 running 中的 attempt 标记为失败。
//
// retryable=true 且未达到 spec.MaxAttempts 时回到 idle、next_due=now（下次
// tick 立即重试）；否则进入 abandoned 终态。同一 token 的重复 fail 幂等。
func Fail(cur Status, spec *Spec, now int64, token, reason string, retryable bool) (Status, bool, error) {
	cur, err := normalize(cur, spec)
	if err != nil {
		return cur, false, err
	}
	if token == "" {
		return cur, false, &Error{Code: CodeInvalid, Message: "token is empty"}
	}
	if cur.Token == token && (cur.Phase == PhaseIdle || cur.Phase == PhaseAbandoned) {
		return cur, false, nil // 重复提交：幂等
	}
	if cur.Phase != PhaseRunning || cur.Token != token {
		return cur, false, &Error{Code: CodeStaleAttempt, Owner: cur.Owner, Message: "token does not own the task"}
	}

	next := cur
	next.Owner = ""
	next.LeaseUntilNanos = 0
	next.LastError = reason
	exhausted := spec != nil && spec.MaxAttempts > 0 && cur.Attempt >= spec.MaxAttempts
	if !retryable || exhausted {
		next.Phase = PhaseAbandoned
		next.NextDueNanos = 0
	} else {
		next.Phase = PhaseIdle
		next.NextDueNanos = now
	}
	return next, true, nil
}

// Renew 延长 running attempt 的租约。租约已过期或 token 不匹配返回
// CodeStaleAttempt——调用方应先重新 claim，而不是无限续命。
func Renew(cur Status, now int64, token string, ttlNanos int64) (Status, bool, error) {
	if ttlNanos <= 0 {
		return cur, false, &Error{Code: CodeInvalid, Message: "ttl must be > 0"}
	}
	cur, err := normalize(cur, nil)
	if err != nil {
		return cur, false, err
	}
	if token == "" || cur.Phase != PhaseRunning || cur.Token != token {
		return cur, false, &Error{Code: CodeStaleAttempt, Owner: cur.Owner, Message: "token does not own the task"}
	}
	if cur.LeaseUntilNanos <= now {
		return cur, false, &Error{Code: CodeStaleAttempt, Owner: cur.Owner, Message: fmt.Sprintf("lease expired at %d", cur.LeaseUntilNanos)}
	}
	next := cur
	next.LeaseUntilNanos = now + ttlNanos
	return next, true, nil
}
