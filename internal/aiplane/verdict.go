package aiplane

import (
	"encoding/json"
	"fmt"
	"sort"
)

// 提议裁决（proposal verdict）是"引擎裁决、agent 只提议"这条纪律的落账点：
// agent 写 proposal:{fingerprint}（不可变），引擎写 verdict:{fingerprint}
// （同样是版本化记录）。裁决不是对 proposal 的修改——proposal 的历史永远
// 保留原样，裁决是另一条独立账本记录，可重放、可审计、可按 as_of 回溯。
//
// 键空间用独立的 verdict: 前缀而不是 proposal:verdict:，原因很实际：
// Context API 要按 proposal: 前缀枚举"有哪些提议"，如果裁决键嵌在同一个
// 前缀下，每次扫描都要显式剔除内部键——把两类对象放在两个前缀下，扫描
// 语义就不需要靠过滤规则维持。

// VerdictDecision 是裁决结论的枚举。用枚举而不是自由字符串：调用方要按
// 结论做程序化分支（是否进入证据关、是否允许再次提议），比较的是值不是文本。
type VerdictDecision string

const (
	// VerdictAccept 采纳提议，允许进入下一阶段（证据关/策略候选）。
	VerdictAccept VerdictDecision = "accept"
	// VerdictReject 否决提议。同一 idea 再次提出时 agent 应看到这条记录。
	VerdictReject VerdictDecision = "reject"
	// VerdictNeedsMoreEvidence 需要补充证据：不是终局结论，agent 可以补数据
	// 后重新提议（新提议内容不同 = 新指纹）。
	VerdictNeedsMoreEvidence VerdictDecision = "needs_more_evidence"
)

var validVerdictDecisions = map[VerdictDecision]bool{
	VerdictAccept:            true,
	VerdictReject:            true,
	VerdictNeedsMoreEvidence: true,
}

// Verdict 是一条提议裁决记录。字段顺序即规范化编码顺序（Verdict 本身没有
// map 或接口字段，json.Marshal 按结构体声明顺序输出，天然确定）。
type Verdict struct {
	// Fingerprint 是被裁决提议的指纹（proposal:{fingerprint} 的键组成部分）。
	Fingerprint string `json:"fingerprint"`
	// Decision 是结论，见 VerdictDecision 常量。
	Decision VerdictDecision `json:"decision"`
	// Rationale 是裁决依据（统计量、证据引用或否决理由），必填——"判决权在
	// 统计不在心情"，一条没有依据的裁决无法审计，与没有裁决等价。
	Rationale string `json:"rationale"`
	// DecidedBy 是裁决方标识（引擎版本/协调者），落审计用。
	DecidedBy string `json:"decided_by"`
	// DecidedAt 由调用方显式提供（业务日期或 as_of 标识）。本包不读系统
	// 时钟：同一份输入重新裁决必须得到逐字节相同的记录。
	DecidedAt string `json:"decided_at,omitempty"`
}

// VerdictValidationError 是裁决字段校验失败的结构化描述。
type VerdictValidationError struct {
	Field  string
	Reason string
}

func (e *VerdictValidationError) Error() string {
	return fmt.Sprintf("verdict 校验失败: 字段=%s 原因=%s", e.Field, e.Reason)
}

// Validate 校验裁决必填字段与枚举成员。
func (v Verdict) Validate() error {
	if v.Fingerprint == "" {
		return &VerdictValidationError{Field: "fingerprint", Reason: "不能为空"}
	}
	if !validVerdictDecisions[v.Decision] {
		return &VerdictValidationError{Field: "decision", Reason: fmt.Sprintf("未知裁决结论: %q", v.Decision)}
	}
	if v.Rationale == "" {
		return &VerdictValidationError{Field: "rationale", Reason: "不能为空（无依据的裁决与没有裁决等价）"}
	}
	if v.DecidedBy == "" {
		return &VerdictValidationError{Field: "decided_by", Reason: "不能为空"}
	}
	return nil
}

// VerdictKey 返回 verdict:{proposal-fingerprint} 逻辑键。
func VerdictKey(proposalFingerprint string) string {
	return "verdict:" + proposalFingerprint
}

// Adjudicate 落一条裁决。约束与 Proposal 对称：
//   - 被裁决的提案必须存在（否则是拼错指纹或凭空裁决，直接拒绝而不是落一条
//     指向无物的记录）；
//   - 同一份裁决内容重复提交是幂等 no-op（返回 seq=0）；
//   - 写入以引擎身份发生（WriteAsEngine），agent 不能自批自己的提议。
//
// 不追加"提案不能重复裁决为不同结论"的门禁：那要读旧裁决再判定，而"同一
// 指纹出现两条不同结论"本身是账本里可见的事实（LIST_VERSIONS），由上层
// 裁决流程负责不自相矛盾；这里不隐藏历史。
func Adjudicate(rw ReadWriter, v Verdict) (uint64, error) {
	if err := v.Validate(); err != nil {
		return 0, err
	}
	existing, found, err := rw.GetAsOf(ProposalKey(v.Fingerprint), nowUnboundedAsOf)
	if err != nil {
		return 0, fmt.Errorf("读被裁决提议失败: %w", err)
	}
	if !found || len(existing) == 0 {
		return 0, &VerdictValidationError{Field: "fingerprint", Reason: "对应提议不存在: " + v.Fingerprint}
	}

	payload, err := json.Marshal(v)
	if err != nil {
		return 0, fmt.Errorf("裁决编码失败: %w", err)
	}
	key := VerdictKey(v.Fingerprint)
	prev, found, err := rw.GetAsOf(key, nowUnboundedAsOf)
	if err != nil {
		return 0, fmt.Errorf("读现有裁决失败: %w", err)
	}
	if found && string(prev) == string(payload) {
		return 0, nil // 幂等：内容完全相同
	}
	return WriteAsEngine(rw, KindVerdict, key, payload)
}

// LookupVerdict 读取某条提议在 asOfNanos 时刻可见的裁决；未裁决返回
// found=false（"还没裁决"不是错误）。
func LookupVerdict(reader AsOfReader, proposalFingerprint string, asOfNanos int64) (Verdict, bool, error) {
	raw, found, err := reader.GetAsOf(VerdictKey(proposalFingerprint), asOfNanos)
	if err != nil || !found {
		return Verdict{}, false, err
	}
	var v Verdict
	if err := json.Unmarshal(raw, &v); err != nil {
		return Verdict{}, false, fmt.Errorf("裁决记录解析失败(%s): %w", proposalFingerprint, err)
	}
	if v.Fingerprint == "" {
		v.Fingerprint = proposalFingerprint
	}
	return v, true, nil
}

// ListVerdicts 返回 asOfNanos 时刻可见的全部裁决，按 fingerprint 升序
// （确定性输出，供 Context 包与审计导出使用）。
func ListVerdicts(lister PrefixLister, asOfNanos int64) ([]Verdict, error) {
	entries, err := lister.ListPrefix(verdictKeyPrefix, asOfNanos)
	if err != nil {
		return nil, fmt.Errorf("扫描 %s 失败: %w", verdictKeyPrefix, err)
	}
	latest := LatestPerLogicalKey(entries)
	out := make([]Verdict, 0, len(latest))
	for _, e := range latest {
		var v Verdict
		if json.Unmarshal(e.Payload, &v) != nil {
			continue // 损坏记录跳过：不是本函数能裁决的事，重放对账会暴露
		}
		if v.Fingerprint == "" {
			v.Fingerprint = e.LogicalKey[len(verdictKeyPrefix):]
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Fingerprint < out[j].Fingerprint })
	return out, nil
}

const verdictKeyPrefix = "verdict:"
