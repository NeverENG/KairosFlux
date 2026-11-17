package ingesthook

import (
	"bytes"
	"testing"

	"github.com/ChronoBrew/KairosFlux/kairnet"
	"github.com/ChronoBrew/KairosFlux/proto"
)

type fakeReq struct {
	msgID string
	data  []byte
}

func (f *fakeReq) Conn() kairnet.Conn  { return nil }
func (f *fakeReq) MsgData() []byte     { return f.data }
func (f *fakeReq) MsgID() string       { return f.msgID }
func (f *fakeReq) SetMsgData(d []byte) { f.data = d }

func putReq(key, value string) *fakeReq {
	return &fakeReq{msgID: proto.MsgPut, data: proto.EncodePutFrame([]byte(key), []byte(value))}
}

func TestHandleRejectsMalformedAndOversizedFrames(t *testing.T) {
	f := NewFilter(4)
	for _, tc := range []struct {
		req    *fakeReq
		reason string
	}{
		{&fakeReq{msgID: proto.MsgPut, data: []byte{1, 2, 3}}, "malformed_frame"},
		{putReq("free:1", "toolong"), "oversized_value"},
	} {
		got, reason := f.Handle(tc.req)
		if got != kairnet.HookDrop || reason != tc.reason {
			t.Fatalf("Handle()=(%v,%q), want HookDrop/%q", got, reason, tc.reason)
		}
	}
	if got, _ := f.Handle(&fakeReq{msgID: proto.MsgGet}); got != kairnet.HookPass {
		t.Fatalf("GET must bypass PUT validation, got %v", got)
	}
}

func TestHandlePreservesResearchPayloadAndNumericSuffixOrder(t *testing.T) {
	f := NewFilter(0)
	for _, key := range []string{"research:run:100", "research:run:99", "research:run:99"} {
		value := []byte(`{"gps":"39.9,116.4","user_id":"u123","note":"original"}`)
		req := putReq(key, string(value))
		frame := bytes.Clone(req.MsgData())
		if got, reason := f.Handle(req); got != kairnet.HookPass {
			t.Fatalf("key=%q: got %v, %s", key, got, reason)
		}
		if !bytes.Equal(req.MsgData(), frame) {
			t.Fatalf("key=%q: write frame was rewritten", key)
		}
		result, _, reason := f.Validate([]byte(key), value)
		if result != ResultPass || reason != "" {
			t.Fatalf("key=%q: payload rejected: %v, %q", key, result, reason)
		}
	}
}

func TestVersionedAndDeclaredTypePreserveValidation(t *testing.T) {
	f := NewFilter(0)
	key := []byte("quote:2026-08-17:600000")
	valid := []byte(`{"code":"600000","date":"2026-08-17","open":10,"high":10.5,"low":9.8,"close":10.2,"volume":1000000}`)
	for i := 0; i < 2; i++ {
		result, _, reason := f.ValidateForType(0, key, valid)
		if result != ResultPass || reason != "" {
			t.Fatalf("versioned write %d rejected: %v %q", i, result, reason)
		}
	}
	if result, _, _ := f.ValidateForType(4242, key, valid); result != ResultDrop {
		t.Fatal("unknown declared type must be rejected")
	}
	if result, _, reason := f.ValidateForType(0, key, []byte(`{"open":-1}`)); result != ResultDrop || reason == "" {
		t.Fatalf("invalid quote must be rejected with reason: %v %q", result, reason)
	}
	if result, _, reason := NewFilter(3).ValidateForType(0, key, valid); result != ResultDrop || reason != "oversized_value" {
		t.Fatalf("oversized versioned value must be rejected: %v %q", result, reason)
	}
}
