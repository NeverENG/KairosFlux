package proto

import (
	"encoding/binary"

	"github.com/ChronoBrew/KairosFlux/internal/taskplane"
)

// TASK_* 请求/响应负载布局（全部小端，版本字节 = 1）。
//
// task plane 的载荷分两层：控制字段手写 TLV（定长字段 + 长度前缀的标量），
// agent 生成的 checkpoint/outcome 作为不透明字节块（长度前缀 u32）随帧携带，
// 内容由上层解释（当前是 JSON）。这样控制语义有字节级契约，agent 内容仍可
// 自由演进，不需要每次加字段都改协议。
//
// 所有解码器对截断帧、未知版本、超限字段返回 ok=false，不做任何"猜测补全"。

const (
	// TaskFrameVersion 是当前 TASK_* 载荷版本；未知版本一律拒绝。
	TaskFrameVersion uint8 = 1

	// MaxTaskNameLen 是任务名上限（含 "task:spec:" 后缀前的业务名）。
	MaxTaskNameLen = 256
	// MaxTaskWorkerLen 是 worker 标识上限。
	MaxTaskWorkerLen = 128
	// MaxTaskTokenLen 是租约令牌上限。
	MaxTaskTokenLen = 64
	// MaxTaskRefs 是一次 checkpoint/complete 携带的引用数量上限。
	MaxTaskRefs = 64
	// MaxTaskRefLen 是单个引用逻辑键上限。
	MaxTaskRefLen = 512
	// MaxTaskBlobLen 是 checkpoint/outcome 字节块上限（1 MiB）。
	MaxTaskBlobLen = 1 << 20
	// MaxTaskReasonLen 是失败原因文本上限。
	MaxTaskReasonLen = 512
)

// TaskClaimRequest 是 TASK_CLAIM 的请求负载。
type TaskClaimRequest struct {
	Name       string
	Worker     string
	TTLSeconds uint32
}

// TaskCheckpointRequest 是 TASK_CHECKPOINT 的请求负载。
type TaskCheckpointRequest struct {
	Name       string
	Token      string
	Checkpoint []byte
	Refs       []string
}

// TaskRenewRequest 是 TASK_RENEW 的请求负载。
type TaskRenewRequest struct {
	Name       string
	Token      string
	TTLSeconds uint32
}

// TaskCompleteRequest 是 TASK_COMPLETE 的请求负载。
type TaskCompleteRequest struct {
	Name       string
	Token      string
	Checkpoint []byte
	Refs       []string
	Outcome    []byte
}

// TaskFailRequest 是 TASK_FAIL 的请求负载。
type TaskFailRequest struct {
	Name      string
	Token     string
	Reason    string
	Retryable bool
}

// EncodeTaskClaimRequest 编 TASK_CLAIM 负载：
// [ver u8][nameLen u16][name][workerLen u16][worker][ttlSeconds u32]。
func EncodeTaskClaimRequest(r TaskClaimRequest) []byte {
	buf := []byte{TaskFrameVersion}
	buf = appendTaskString(buf, r.Name)
	buf = appendTaskString(buf, r.Worker)
	buf = binary.LittleEndian.AppendUint32(buf, r.TTLSeconds)
	return buf
}

// DecodeTaskClaimRequest 解析 TASK_CLAIM 负载，失败返回 ok=false。
func DecodeTaskClaimRequest(data []byte) (TaskClaimRequest, bool) {
	var r TaskClaimRequest
	rd := taskReader{buf: data}
	if rd.version() == false {
		return r, false
	}
	var ok bool
	if r.Name, ok = rd.str16(MaxTaskNameLen); !ok {
		return r, false
	}
	if r.Worker, ok = rd.str16(MaxTaskWorkerLen); !ok {
		return r, false
	}
	if r.TTLSeconds, ok = rd.u32(); !ok {
		return r, false
	}
	return r, rd.done()
}

// EncodeTaskCheckpointRequest 编 TASK_CHECKPOINT 负载：
// [ver][name][token][checkpoint blob u32][refs]。
func EncodeTaskCheckpointRequest(r TaskCheckpointRequest) []byte {
	buf := []byte{TaskFrameVersion}
	buf = appendTaskString(buf, r.Name)
	buf = appendTaskString(buf, r.Token)
	buf = appendTaskBlob(buf, r.Checkpoint)
	buf = appendTaskRefs(buf, r.Refs)
	return buf
}

// DecodeTaskCheckpointRequest 解析 TASK_CHECKPOINT 负载。
func DecodeTaskCheckpointRequest(data []byte) (TaskCheckpointRequest, bool) {
	var r TaskCheckpointRequest
	rd := taskReader{buf: data}
	if rd.version() == false {
		return r, false
	}
	var ok bool
	if r.Name, ok = rd.str16(MaxTaskNameLen); !ok {
		return r, false
	}
	if r.Token, ok = rd.str16(MaxTaskTokenLen); !ok {
		return r, false
	}
	if r.Checkpoint, ok = rd.blob32(MaxTaskBlobLen); !ok {
		return r, false
	}
	if r.Refs, ok = rd.refs16(); !ok {
		return r, false
	}
	return r, rd.done()
}

// EncodeTaskRenewRequest 编 TASK_RENEW 负载：
// [ver][name][token][ttlSeconds u32]。
func EncodeTaskRenewRequest(r TaskRenewRequest) []byte {
	buf := []byte{TaskFrameVersion}
	buf = appendTaskString(buf, r.Name)
	buf = appendTaskString(buf, r.Token)
	buf = binary.LittleEndian.AppendUint32(buf, r.TTLSeconds)
	return buf
}

// DecodeTaskRenewRequest 解析 TASK_RENEW 负载。
func DecodeTaskRenewRequest(data []byte) (TaskRenewRequest, bool) {
	var r TaskRenewRequest
	rd := taskReader{buf: data}
	if rd.version() == false {
		return r, false
	}
	var ok bool
	if r.Name, ok = rd.str16(MaxTaskNameLen); !ok {
		return r, false
	}
	if r.Token, ok = rd.str16(MaxTaskTokenLen); !ok {
		return r, false
	}
	if r.TTLSeconds, ok = rd.u32(); !ok {
		return r, false
	}
	return r, rd.done()
}

// EncodeTaskCompleteRequest 编 TASK_COMPLETE 负载：
// [ver][name][token][checkpoint blob][refs][outcome blob]。
func EncodeTaskCompleteRequest(r TaskCompleteRequest) []byte {
	buf := []byte{TaskFrameVersion}
	buf = appendTaskString(buf, r.Name)
	buf = appendTaskString(buf, r.Token)
	buf = appendTaskBlob(buf, r.Checkpoint)
	buf = appendTaskRefs(buf, r.Refs)
	buf = appendTaskBlob(buf, r.Outcome)
	return buf
}

// DecodeTaskCompleteRequest 解析 TASK_COMPLETE 负载。
func DecodeTaskCompleteRequest(data []byte) (TaskCompleteRequest, bool) {
	var r TaskCompleteRequest
	rd := taskReader{buf: data}
	if rd.version() == false {
		return r, false
	}
	var ok bool
	if r.Name, ok = rd.str16(MaxTaskNameLen); !ok {
		return r, false
	}
	if r.Token, ok = rd.str16(MaxTaskTokenLen); !ok {
		return r, false
	}
	if r.Checkpoint, ok = rd.blob32(MaxTaskBlobLen); !ok {
		return r, false
	}
	if r.Refs, ok = rd.refs16(); !ok {
		return r, false
	}
	if r.Outcome, ok = rd.blob32(MaxTaskBlobLen); !ok {
		return r, false
	}
	return r, rd.done()
}

// EncodeTaskFailRequest 编 TASK_FAIL 负载：
// [ver][name][token][retryable u8][reasonLen u16][reason]。
func EncodeTaskFailRequest(r TaskFailRequest) []byte {
	buf := []byte{TaskFrameVersion}
	buf = appendTaskString(buf, r.Name)
	buf = appendTaskString(buf, r.Token)
	if r.Retryable {
		buf = append(buf, 1)
	} else {
		buf = append(buf, 0)
	}
	buf = appendTaskString(buf, r.Reason)
	return buf
}

// DecodeTaskFailRequest 解析 TASK_FAIL 负载。
func DecodeTaskFailRequest(data []byte) (TaskFailRequest, bool) {
	var r TaskFailRequest
	rd := taskReader{buf: data}
	if rd.version() == false {
		return r, false
	}
	var ok bool
	if r.Name, ok = rd.str16(MaxTaskNameLen); !ok {
		return r, false
	}
	if r.Token, ok = rd.str16(MaxTaskTokenLen); !ok {
		return r, false
	}
	flag, ok := rd.u8()
	if !ok {
		return r, false
	}
	r.Retryable = flag == 1
	if r.Reason, ok = rd.str16(MaxTaskReasonLen); !ok {
		return r, false
	}
	return r, rd.done()
}

// EncodeTaskStatus 编任务状态（TASK_* 成功响应负载）：
// [ver][name][phase u8][owner][attempt u32][token][leaseUntil i64][nextDue i64]
// [checkpoint blob][refs][outcome blob][lastError]。
func EncodeTaskStatus(s taskplane.Status) []byte {
	buf := []byte{TaskFrameVersion}
	buf = appendTaskString(buf, s.Name)
	buf = append(buf, taskPhaseCode(s.Phase))
	buf = appendTaskString(buf, s.Owner)
	buf = binary.LittleEndian.AppendUint32(buf, s.Attempt)
	buf = appendTaskString(buf, s.Token)
	buf = binary.LittleEndian.AppendUint64(buf, uint64(s.LeaseUntilNanos))
	buf = binary.LittleEndian.AppendUint64(buf, uint64(s.NextDueNanos))
	buf = appendTaskBlob(buf, s.Checkpoint)
	buf = appendTaskRefs(buf, s.Refs)
	buf = appendTaskBlob(buf, s.Outcome)
	buf = appendTaskString(buf, s.LastError)
	return buf
}

// DecodeTaskStatus 解析任务状态；未知阶段或截断帧返回 ok=false。
func DecodeTaskStatus(data []byte) (taskplane.Status, bool) {
	var s taskplane.Status
	rd := taskReader{buf: data}
	if rd.version() == false {
		return s, false
	}
	var ok bool
	if s.Name, ok = rd.str16(MaxTaskNameLen); !ok {
		return s, false
	}
	code, ok := rd.u8()
	if !ok {
		return s, false
	}
	if s.Phase, ok = taskPhaseFromCode(code); !ok {
		return s, false
	}
	if s.Owner, ok = rd.str16(MaxTaskWorkerLen); !ok {
		return s, false
	}
	if s.Attempt, ok = rd.u32(); !ok {
		return s, false
	}
	if s.Token, ok = rd.str16(MaxTaskTokenLen); !ok {
		return s, false
	}
	lease, ok := rd.u64()
	if !ok {
		return s, false
	}
	due, ok := rd.u64()
	if !ok {
		return s, false
	}
	s.LeaseUntilNanos = int64(lease)
	s.NextDueNanos = int64(due)
	if s.Checkpoint, ok = rd.blob32(MaxTaskBlobLen); !ok {
		return s, false
	}
	if s.Refs, ok = rd.refs16(); !ok {
		return s, false
	}
	if s.Outcome, ok = rd.blob32(MaxTaskBlobLen); !ok {
		return s, false
	}
	if s.LastError, ok = rd.str16(MaxTaskReasonLen); !ok {
		return s, false
	}
	return s, rd.done()
}

// taskPhaseCode 把阶段映射为线上的 u8；未知阶段映射为 0（调用方应保证有效）。
func taskPhaseCode(p taskplane.Phase) uint8 {
	switch p {
	case taskplane.PhaseIdle:
		return 1
	case taskplane.PhaseRunning:
		return 2
	case taskplane.PhaseSucceeded:
		return 3
	case taskplane.PhaseFailed:
		return 4
	case taskplane.PhaseAbandoned:
		return 5
	}
	return 0
}

func taskPhaseFromCode(code uint8) (taskplane.Phase, bool) {
	switch code {
	case 1:
		return taskplane.PhaseIdle, true
	case 2:
		return taskplane.PhaseRunning, true
	case 3:
		return taskplane.PhaseSucceeded, true
	case 4:
		return taskplane.PhaseFailed, true
	case 5:
		return taskplane.PhaseAbandoned, true
	}
	return "", false
}

func appendTaskString(buf []byte, s string) []byte {
	buf = binary.LittleEndian.AppendUint16(buf, uint16(len(s)))
	return append(buf, s...)
}

func appendTaskBlob(buf, blob []byte) []byte {
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(blob)))
	return append(buf, blob...)
}

func appendTaskRefs(buf []byte, refs []string) []byte {
	buf = binary.LittleEndian.AppendUint16(buf, uint16(len(refs)))
	for _, ref := range refs {
		buf = appendTaskString(buf, ref)
	}
	return buf
}

// taskReader 是带越界检查的小端读取游标；任何一次读取越界都会置 ok=false，
// 后续读取保持失败，最终 done() 返回 false。
type taskReader struct {
	buf []byte
	off int
	bad bool
}

func (r *taskReader) take(n int) []byte {
	if r.bad || n < 0 || r.off+n > len(r.buf) {
		r.bad = true
		return nil
	}
	out := r.buf[r.off : r.off+n]
	r.off += n
	return out
}

func (r *taskReader) version() bool {
	b, ok := r.u8()
	return ok && b == TaskFrameVersion
}

func (r *taskReader) u8() (uint8, bool) {
	b := r.take(1)
	if b == nil {
		return 0, false
	}
	return b[0], true
}

func (r *taskReader) u16() (uint16, bool) {
	b := r.take(2)
	if b == nil {
		return 0, false
	}
	return binary.LittleEndian.Uint16(b), true
}

func (r *taskReader) u32() (uint32, bool) {
	b := r.take(4)
	if b == nil {
		return 0, false
	}
	return binary.LittleEndian.Uint32(b), true
}

func (r *taskReader) u64() (uint64, bool) {
	b := r.take(8)
	if b == nil {
		return 0, false
	}
	return binary.LittleEndian.Uint64(b), true
}

func (r *taskReader) str16(max int) (string, bool) {
	n, ok := r.u16()
	if !ok || int(n) > max {
		r.bad = true
		return "", false
	}
	b := r.take(int(n))
	if b == nil {
		return "", false
	}
	return string(b), true
}

func (r *taskReader) blob32(max int) ([]byte, bool) {
	n, ok := r.u32()
	if !ok || int(n) > max {
		r.bad = true
		return nil, false
	}
	b := r.take(int(n))
	if b == nil {
		return nil, false
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out, true
}

func (r *taskReader) refs16() ([]string, bool) {
	n, ok := r.u16()
	if !ok || int(n) > MaxTaskRefs {
		r.bad = true
		return nil, false
	}
	refs := make([]string, 0, n)
	for range int(n) {
		ref, ok := r.str16(MaxTaskRefLen)
		if !ok {
			return nil, false
		}
		refs = append(refs, ref)
	}
	return refs, true
}

func (r *taskReader) done() bool {
	return !r.bad && r.off == len(r.buf)
}
