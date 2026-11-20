package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ChronoBrew/KairosFlux/internal/taskplane"
)

// TaskStore 是 task plane 的 IO 壳：把纯语义（internal/taskplane）接到版本化
// 存储上。每次状态迁移都是"读最新版本 → 纯函数迁移 → 条件追加（CAS）"，
// 因此并发 claim 恰好一个赢家，崩溃后下一次 tick 从账本继续。
//
// 时间与 token 都通过字段注入：生产用 time.Now 与 crypto/rand，测试用假时钟
// 和确定性序号，租约/过期语义不依赖 sleep。
type TaskStore struct {
	temporal *TemporalStore
	now      func() time.Time
	token    func() (string, error)
}

const (
	taskPlaneSource     = "taskplane"
	taskStoreCASRetries = 16
)

// NewTaskStore 用生产时钟与随机 token 构造 TaskStore。
func NewTaskStore(t *TemporalStore) *TaskStore {
	return newTaskStore(t, time.Now, newTaskToken)
}

func newTaskStore(t *TemporalStore, now func() time.Time, token func() (string, error)) *TaskStore {
	return &TaskStore{temporal: t, now: now, token: token}
}

func newTaskToken() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("taskplane: generate token: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

// DefineSpec 写入一份任务规格的新版本（规格本身也是版本化记录，历史可审计）。
func (s *TaskStore) DefineSpec(spec taskplane.Spec) (uint64, error) {
	if err := spec.Validate(); err != nil {
		return 0, err
	}
	payload, err := json.Marshal(spec)
	if err != nil {
		return 0, err
	}
	return s.temporal.PutVersioned(taskplane.SpecKey(spec.Name), payload, s.now().UnixNano(), taskPlaneSource, 0)
}

// Claim 取得一次执行租约。语义见 taskplane.Claim：leased / not_due / abandoned
// 以 *taskplane.Error 返回，调用方按 Code 分支。
func (s *TaskStore) Claim(name, worker string, ttl time.Duration) (taskplane.Status, error) {
	if name == "" {
		return taskplane.Status{}, &taskplane.Error{Code: taskplane.CodeInvalid, Message: "task name is empty"}
	}
	if ttl <= 0 {
		return taskplane.Status{}, &taskplane.Error{Code: taskplane.CodeInvalid, Message: "ttl must be > 0"}
	}
	token, err := s.token()
	if err != nil {
		return taskplane.Status{}, err
	}
	return s.mutate(name, func(cur taskplane.Status, spec *taskplane.Spec, now int64) (taskplane.Status, bool, error) {
		return taskplane.Claim(cur, spec, now, worker, token, int64(ttl))
	})
}

// Checkpoint 保存当前 attempt 的断点与引用。
func (s *TaskStore) Checkpoint(name, token string, checkpoint []byte, refs []string) (taskplane.Status, error) {
	return s.taskMutation(name, func(cur taskplane.Status, _ *taskplane.Spec, now int64) (taskplane.Status, bool, error) {
		return taskplane.Checkpoint(cur, now, token, checkpoint, refs)
	})
}

// Renew 延长当前 attempt 的租约。
func (s *TaskStore) Renew(name, token string, ttl time.Duration) (taskplane.Status, error) {
	if ttl <= 0 {
		return taskplane.Status{}, &taskplane.Error{Code: taskplane.CodeInvalid, Message: "ttl must be > 0"}
	}
	return s.taskMutation(name, func(cur taskplane.Status, _ *taskplane.Spec, now int64) (taskplane.Status, bool, error) {
		return taskplane.Renew(cur, now, token, int64(ttl))
	})
}

// Complete 提交成功结果；next_due 由 spec 的 cadence 决定。
func (s *TaskStore) Complete(name, token string, done taskplane.Completion) (taskplane.Status, error) {
	return s.taskMutation(name, func(cur taskplane.Status, spec *taskplane.Spec, now int64) (taskplane.Status, bool, error) {
		return taskplane.Complete(cur, spec, now, token, done)
	})
}

// Fail 提交失败；retryable 且未达重试上限时回到 idle 等待下一次 claim。
func (s *TaskStore) Fail(name, token, reason string, retryable bool) (taskplane.Status, error) {
	return s.taskMutation(name, func(cur taskplane.Status, spec *taskplane.Spec, now int64) (taskplane.Status, bool, error) {
		return taskplane.Fail(cur, spec, now, token, reason, retryable)
	})
}

// Status 返回任务当前状态；found=false 表示从未写入过。
func (s *TaskStore) Status(name string) (taskplane.Status, bool, error) {
	cur, _, seq, err := s.load(name)
	if err != nil {
		return taskplane.Status{}, false, err
	}
	return cur, seq > 0, nil
}

func (s *TaskStore) taskMutation(name string, fn func(taskplane.Status, *taskplane.Spec, int64) (taskplane.Status, bool, error)) (taskplane.Status, error) {
	if name == "" {
		return taskplane.Status{}, &taskplane.Error{Code: taskplane.CodeInvalid, Message: "task name is empty"}
	}
	return s.mutate(name, fn)
}

// mutate 执行一次 CAS 循环：纯函数不改变状态时直接返回；改变时条件追加，
// 追加失败（别的迁移先落盘）则重读重算。最多 taskStoreCASRetries 次。
func (s *TaskStore) mutate(name string, fn func(taskplane.Status, *taskplane.Spec, int64) (taskplane.Status, bool, error)) (taskplane.Status, error) {
	for range taskStoreCASRetries {
		cur, spec, seq, err := s.load(name)
		if err != nil {
			return taskplane.Status{}, err
		}
		now := s.now().UnixNano()
		next, changed, err := fn(cur, spec, now)
		if err != nil {
			return taskplane.Status{}, err
		}
		if !changed {
			return next, nil
		}
		payload, err := json.Marshal(next)
		if err != nil {
			return taskplane.Status{}, err
		}
		_, ok, err := s.temporal.PutVersionedIf(taskplane.StatusKey(name), seq, payload, now, taskPlaneSource, 0)
		if err != nil {
			return taskplane.Status{}, err
		}
		if ok {
			return next, nil
		}
	}
	return taskplane.Status{}, fmt.Errorf("taskplane: status contention after %d retries", taskStoreCASRetries)
}

// load 读取任务的最新状态与规格。状态不存在时返回 Initial（seq=0），
// 这样首次 claim 走同一条迁移路径，不需要专门的初始化分支。
func (s *TaskStore) load(name string) (taskplane.Status, *taskplane.Spec, uint64, error) {
	spec, err := s.loadSpec(name)
	if err != nil {
		return taskplane.Status{}, nil, 0, err
	}
	versions, err := s.temporal.ListVersions(taskplane.StatusKey(name))
	if err != nil {
		return taskplane.Status{}, nil, 0, err
	}
	if len(versions) == 0 {
		return taskplane.Initial(name, spec), spec, 0, nil
	}
	last := versions[len(versions)-1]
	var st taskplane.Status
	if err := json.Unmarshal(last.Payload, &st); err != nil {
		return taskplane.Status{}, nil, 0, fmt.Errorf("taskplane: decode status %q: %w", name, err)
	}
	if st.Name == "" {
		st.Name = name
	}
	return st, spec, last.Seq, nil
}

// loadSpec 读取任务规格的最新版本；未定义返回 nil（无规格 = 立即到期、
// 不限重试、完成后立即再次到期）。
func (s *TaskStore) loadSpec(name string) (*taskplane.Spec, error) {
	versions, err := s.temporal.ListVersions(taskplane.SpecKey(name))
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, nil
	}
	var spec taskplane.Spec
	if err := json.Unmarshal(versions[len(versions)-1].Payload, &spec); err != nil {
		return nil, fmt.Errorf("taskplane: decode spec %q: %w", name, err)
	}
	if spec.Name == "" {
		spec.Name = name
	}
	return &spec, nil
}
