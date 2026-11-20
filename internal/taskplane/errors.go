package taskplane

import "fmt"

// Code 是任务迁移错误的结构化分类。它对应协议层的错误码段（0x5xxx），
// 客户端据此决定重试/放弃，而不是解析文本。
type Code string

const (
	// CodeLeased：任务正在被其他 attempt 持有且租约未过期。
	CodeLeased Code = "leased"
	// CodeNotDue：任务尚未到 next_due，拒绝提前 claim。
	CodeNotDue Code = "not_due"
	// CodeStaleAttempt：token 不匹配或阶段不是 running（租约已被接管/过期）。
	CodeStaleAttempt Code = "stale_attempt"
	// CodeAbandoned：任务已进入终态或达到重试上限。
	CodeAbandoned Code = "abandoned"
	// CodeInvalid：输入不合法（空名称/空 worker/非正 TTL 等）。
	CodeInvalid Code = "invalid"
)

// Error 是任务迁移失败的结构化错误。Owner/LeaseUntilNanos/NextDueNanos
// 只在对应 Code 下有值，其余为零值。
type Error struct {
	Code            Code
	Owner           string
	LeaseUntilNanos int64
	NextDueNanos    int64
	Message         string
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("taskplane: %s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("taskplane: %s", e.Code)
}
