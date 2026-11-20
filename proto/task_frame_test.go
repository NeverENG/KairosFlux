package proto

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ChronoBrew/KairosFlux/internal/taskplane"
)

func TestTaskClaimRequestRoundTrip(t *testing.T) {
	in := TaskClaimRequest{Name: "research:daily", Worker: "worker-1", TTLSeconds: 300}
	out, ok := DecodeTaskClaimRequest(EncodeTaskClaimRequest(in))
	if !ok || out != in {
		t.Fatalf("round trip failed: %+v ok=%v", out, ok)
	}
}

func TestTaskCheckpointRequestRoundTrip(t *testing.T) {
	in := TaskCheckpointRequest{
		Name:       "research:daily",
		Token:      "tok-1",
		Checkpoint: []byte(`{"cursor":42}`),
		Refs:       []string{"proposal:abc", "evidence:1"},
	}
	out, ok := DecodeTaskCheckpointRequest(EncodeTaskCheckpointRequest(in))
	if !ok || out.Name != in.Name || out.Token != in.Token || !bytes.Equal(out.Checkpoint, in.Checkpoint) || len(out.Refs) != 2 {
		t.Fatalf("round trip failed: %+v ok=%v", out, ok)
	}
}

func TestTaskRenewRequestRoundTrip(t *testing.T) {
	in := TaskRenewRequest{Name: "research:daily", Token: "tok-1", TTLSeconds: 60}
	out, ok := DecodeTaskRenewRequest(EncodeTaskRenewRequest(in))
	if !ok || out != in {
		t.Fatalf("round trip failed: %+v ok=%v", out, ok)
	}
}

func TestTaskCompleteRequestRoundTrip(t *testing.T) {
	in := TaskCompleteRequest{
		Name:       "research:daily",
		Token:      "tok-1",
		Checkpoint: []byte(`{"done":true}`),
		Refs:       []string{"proposal:abc"},
		Outcome:    []byte(`{"ok":true}`),
	}
	out, ok := DecodeTaskCompleteRequest(EncodeTaskCompleteRequest(in))
	if !ok || !bytes.Equal(out.Outcome, in.Outcome) || out.Name != in.Name || out.Token != in.Token {
		t.Fatalf("round trip failed: %+v ok=%v", out, ok)
	}
}

func TestTaskFailRequestRoundTrip(t *testing.T) {
	in := TaskFailRequest{Name: "research:daily", Token: "tok-1", Reason: "boom", Retryable: true}
	out, ok := DecodeTaskFailRequest(EncodeTaskFailRequest(in))
	if !ok || out != in {
		t.Fatalf("round trip failed: %+v ok=%v", out, ok)
	}
}

func TestTaskStatusRoundTrip(t *testing.T) {
	in := taskplane.Status{
		Name:            "research:daily",
		Phase:           taskplane.PhaseRunning,
		Owner:           "worker-1",
		Attempt:         3,
		Token:           "tok-1",
		LeaseUntilNanos: 12345,
		NextDueNanos:    67890,
		Checkpoint:      []byte(`{"cursor":42}`),
		Refs:            []string{"proposal:abc"},
		Outcome:         []byte(`{"ok":true}`),
		LastError:       "retry me",
	}
	out, ok := DecodeTaskStatus(EncodeTaskStatus(in))
	if !ok {
		t.Fatal("decode failed")
	}
	if out.Name != in.Name || out.Phase != in.Phase || out.Owner != in.Owner || out.Attempt != in.Attempt ||
		out.Token != in.Token || out.LeaseUntilNanos != in.LeaseUntilNanos || out.NextDueNanos != in.NextDueNanos ||
		!bytes.Equal(out.Checkpoint, in.Checkpoint) || len(out.Refs) != len(in.Refs) || !bytes.Equal(out.Outcome, in.Outcome) ||
		out.LastError != in.LastError {
		t.Fatalf("status mismatch: %+v", out)
	}
}

func TestTaskFramesRejectTruncatedAndUnknownVersion(t *testing.T) {
	frames := map[string][]byte{
		"claim":      EncodeTaskClaimRequest(TaskClaimRequest{Name: "n", Worker: "w", TTLSeconds: 1}),
		"checkpoint": EncodeTaskCheckpointRequest(TaskCheckpointRequest{Name: "n", Token: "t"}),
		"renew":      EncodeTaskRenewRequest(TaskRenewRequest{Name: "n", Token: "t", TTLSeconds: 1}),
		"complete":   EncodeTaskCompleteRequest(TaskCompleteRequest{Name: "n", Token: "t"}),
		"fail":       EncodeTaskFailRequest(TaskFailRequest{Name: "n", Token: "t", Reason: "e"}),
	}
	for name, frame := range frames {
		if len(frame) < 2 {
			t.Fatalf("%s: frame too short", name)
		}
		for cut := 0; cut < len(frame); cut++ {
			if _, ok := decodeTaskFrame(name, frame[:cut]); ok {
				t.Fatalf("%s: truncated frame accepted at %d", name, cut)
			}
		}
	}
	if _, ok := DecodeTaskClaimRequest(append([]byte{0xEE}, EncodeTaskClaimRequest(TaskClaimRequest{Name: "n", Worker: "w", TTLSeconds: 1})[1:]...)); ok {
		t.Fatal("unknown version accepted")
	}
}

func TestTaskFramesEnforceNameLengthCap(t *testing.T) {
	long := strings.Repeat("x", MaxTaskNameLen+1)
	if _, ok := DecodeTaskClaimRequest(EncodeTaskClaimRequest(TaskClaimRequest{Name: long, Worker: "w", TTLSeconds: 1})); ok {
		t.Fatal("oversized name must be rejected")
	}
}

func TestTaskStatusRejectsUnknownPhase(t *testing.T) {
	raw := EncodeTaskStatus(taskplane.Status{Name: "n", Phase: taskplane.PhaseIdle})
	// ver(1) + nameLen(2) + name(1) → phase 在第 4 字节。
	raw[4] = 0xEE
	if _, ok := DecodeTaskStatus(raw); ok {
		t.Fatal("unknown phase must be rejected")
	}
}

// decodeTaskFrame 是测试用的按类型分派的解码器，验证截断帧不会 panic 或被接受。
func decodeTaskFrame(name string, frame []byte) (any, bool) {
	switch name {
	case "claim":
		return DecodeTaskClaimRequest(frame)
	case "checkpoint":
		return DecodeTaskCheckpointRequest(frame)
	case "renew":
		return DecodeTaskRenewRequest(frame)
	case "complete":
		return DecodeTaskCompleteRequest(frame)
	case "fail":
		return DecodeTaskFailRequest(frame)
	}
	return nil, false
}
