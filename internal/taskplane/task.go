// Package taskplane 定义研究任务进度（task plane）的纯语义：任务规格、
// 状态机与迁移规则。
//
// 设计约束（与 internal/temporal 同构）：
//   - 本包不碰存储、网络、时钟与随机数：所有迁移函数把 now/token 作为参数，
//     因此可以穷举测试，也可以在任意进程里以相同输入得到逐字节相同的状态。
//   - 任务状态本身是版本化记录：每次迁移 = 一次新版本，历史即事件日志。
//     本包只定义"迁移是否合法、结果是什么"，落盘由 service.TaskStore 负责。
//   - 没有后台清理任务：租约过期是读时判定（running 且 lease_until <= now
//     即可被重新 claim），不引入 sweeper goroutine。
//
// 状态机：
//
//	idle ──claim──▶ running ──complete──▶ succeeded ──due──▶ claim ...
//	                  │  └──fail(retry)──▶ idle
//	                  └─────fail(final)──▶ abandoned（终态，不再可 claim）
package taskplane

import "fmt"

// Phase 是任务的当前阶段。取值是协议的一部分，不得随意更名。
type Phase string

const (
	// PhaseIdle 表示任务可被 claim（受 next_due 约束）。
	PhaseIdle Phase = "idle"
	// PhaseRunning 表示某次 attempt 正在执行并持有租约。
	PhaseRunning Phase = "running"
	// PhaseSucceeded 表示最近一次 attempt 成功；到 next_due 后可再次 claim。
	PhaseSucceeded Phase = "succeeded"
	// PhaseFailed 预留给"已失败但保留可重试语义"的显式状态；当前 Fail 的
	// 可重试路径直接回到 idle，不经过该阶段。
	PhaseFailed Phase = "failed"
	// PhaseAbandoned 是终态：达到重试上限或不可重试的失败，不再可 claim。
	PhaseAbandoned Phase = "abandoned"
)

// Valid 报告 p 是否是已知阶段。
func (p Phase) Valid() bool {
	switch p {
	case PhaseIdle, PhaseRunning, PhaseSucceeded, PhaseFailed, PhaseAbandoned:
		return true
	}
	return false
}

// Spec 是任务的声明式规格，存于 task:spec:<name> 的版本化记录中。
// CadenceNanos <= 0 表示不定期任务（完成后 next_due=0，立即再次到期）。
// MaxAttempts == 0 表示不限制重试次数。
type Spec struct {
	Name          string `json:"name"`
	Kind          string `json:"kind,omitempty"`
	CadenceNanos  int64  `json:"cadence_nanos,omitempty"`
	FirstDueNanos int64  `json:"first_due_nanos,omitempty"`
	MaxAttempts   uint32 `json:"max_attempts,omitempty"`
}

// Validate 校验规格的自洽性；不做 IO。
func (s Spec) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("taskplane: spec name is empty")
	}
	if s.CadenceNanos < 0 {
		return fmt.Errorf("taskplane: spec cadence_nanos must be >= 0")
	}
	if s.FirstDueNanos < 0 {
		return fmt.Errorf("taskplane: spec first_due_nanos must be >= 0")
	}
	return nil
}

// Status 是任务的一个不可变版本（存于 task:status:<name>）。字段全部是可
// 序列化的标量或字节块，便于跨语言（Go/Python/Rust）一致实现。
type Status struct {
	Name            string   `json:"name"`
	Phase           Phase    `json:"phase"`
	Owner           string   `json:"owner,omitempty"`
	Attempt         uint32   `json:"attempt,omitempty"`
	Token           string   `json:"token,omitempty"`
	LeaseUntilNanos int64    `json:"lease_until_nanos,omitempty"`
	NextDueNanos    int64    `json:"next_due_nanos,omitempty"`
	Checkpoint      []byte   `json:"checkpoint,omitempty"`
	Refs            []string `json:"refs,omitempty"`
	Outcome         []byte   `json:"outcome,omitempty"`
	LastError       string   `json:"last_error,omitempty"`
}

// Completion 是 complete 迁移的输入：断点、引用与结果。约定：
//   - Checkpoint 是下一次 attempt 继续执行所需的最小状态（不透明字节）；
//   - Refs 是 proposal/evidence/context 的逻辑键，只引用不复制内容；
//   - Outcome 是本次执行的结果（不透明字节）。
type Completion struct {
	Checkpoint []byte
	Refs       []string
	Outcome    []byte
}

// 任务对象在账本里的键空间前缀。它们是跨语言契约的一部分：PrefixLister
// 类读取（如 Context 组装）按这些前缀扫描，不会靠字符串字面量各自维护。
const (
	SpecKeyPrefix   = "task:spec:"
	StatusKeyPrefix = "task:status:"
)

// SpecKey / StatusKey 是任务对象在账本里的逻辑键。
func SpecKey(name string) string   { return SpecKeyPrefix + name }
func StatusKey(name string) string { return StatusKeyPrefix + name }

// Terminal 报告该阶段是否处于"休息态"（不会再被当前 attempt 改变）。
func (s Status) Terminal() bool {
	return s.Phase == PhaseSucceeded || s.Phase == PhaseFailed || s.Phase == PhaseAbandoned
}

// Initial 返回任务从未落盘时的初始状态：phase=idle，next_due 取 spec 的
// 首次到期时间（无 spec 时立即到期）。
func Initial(name string, spec *Spec) Status {
	next := int64(0)
	if spec != nil {
		next = spec.FirstDueNanos
	}
	return Status{Name: name, Phase: PhaseIdle, NextDueNanos: next}
}

// normalize 校验并补全 cur/spec 的名称一致性。返回补齐后的 Status。
func normalize(cur Status, spec *Spec) (Status, error) {
	if spec != nil {
		if err := spec.Validate(); err != nil {
			return cur, err
		}
	}
	if cur.Name == "" {
		if spec == nil {
			return cur, &Error{Code: CodeInvalid, Message: "task name is empty"}
		}
		cur.Name = spec.Name
	}
	if spec != nil && spec.Name != cur.Name {
		return cur, &Error{Code: CodeInvalid, Message: fmt.Sprintf("spec name %q != status name %q", spec.Name, cur.Name)}
	}
	if cur.Phase == "" {
		cur.Phase = PhaseIdle
	}
	if !cur.Phase.Valid() {
		return cur, &Error{Code: CodeInvalid, Message: fmt.Sprintf("unknown phase %q", cur.Phase)}
	}
	return cur, nil
}

func nextDueAfter(spec *Spec, now int64) int64 {
	if spec != nil && spec.CadenceNanos > 0 {
		return now + spec.CadenceNanos
	}
	return 0
}
