package aiplane

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ChronoBrew/KairosFlux/internal/taskplane"
)

func submitTestProposal(t *testing.T, store *fakeReadWriter, summary string) string {
	t.Helper()
	fp, _, err := SubmitProposal(store, Proposal{
		Kind:        ProposalHypothesis,
		SubmittedBy: "researcher-v1",
		Summary:     summary,
	})
	if err != nil {
		t.Fatalf("SubmitProposal 失败: %v", err)
	}
	return fp
}

func TestAdjudicateRequiresExistingProposal(t *testing.T) {
	store := newFakeReadWriter()
	_, err := Adjudicate(store, Verdict{
		Fingerprint: "deadbeef",
		Decision:    VerdictReject,
		Rationale:   "无对应提议",
		DecidedBy:   "engine",
	})
	var ve *VerdictValidationError
	if !errors.As(err, &ve) || ve.Field != "fingerprint" {
		t.Fatalf("期望 fingerprint 校验失败，实际 %v", err)
	}
}

func TestAdjudicatePersistsAndIsIdempotent(t *testing.T) {
	store := newFakeReadWriter()
	fp := submitTestProposal(t, store, "低流动性个股未来超额收益更高")

	v := Verdict{
		Fingerprint: fp,
		Decision:    VerdictAccept,
		Rationale:   "p_global=0.03 通过族内校正",
		DecidedBy:   "engine",
		DecidedAt:   "2026-11-21",
	}
	seq, err := Adjudicate(store, v)
	if err != nil || seq == 0 {
		t.Fatalf("首次裁决应落一次写入: seq=%d err=%v", seq, err)
	}

	// 内容完全相同 → 幂等 no-op（不产生新版本）。
	got, found, err := LookupVerdict(store, fp, nowUnboundedAsOf)
	if err != nil || !found || got.Decision != VerdictAccept || got.Rationale != v.Rationale {
		t.Fatalf("裁决读取失败: %+v found=%v err=%v", got, found, err)
	}
	seq3, err := Adjudicate(store, v)
	if err != nil || seq3 != 0 {
		t.Fatalf("重复裁决应幂等: seq=%d err=%v", seq3, err)
	}
}

func TestAdjudicateRejectsIncompleteVerdict(t *testing.T) {
	store := newFakeReadWriter()
	fp := submitTestProposal(t, store, "假设")
	cases := []Verdict{
		{Fingerprint: fp, Decision: "", Rationale: "r", DecidedBy: "engine"},
		{Fingerprint: fp, Decision: "maybe", Rationale: "r", DecidedBy: "engine"},
		{Fingerprint: fp, Decision: VerdictAccept, Rationale: "", DecidedBy: "engine"},
		{Fingerprint: fp, Decision: VerdictAccept, Rationale: "r", DecidedBy: ""},
	}
	for _, v := range cases {
		var ve *VerdictValidationError
		if _, err := Adjudicate(store, v); !errors.As(err, &ve) {
			t.Fatalf("不完整裁决应被拒绝: %+v err=%v", v, err)
		}
	}
}

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

func TestListVerdictsSortedAndAsOfBounded(t *testing.T) {
	store := newFakeReadWriter()
	fpA := submitTestProposal(t, store, "A")
	fpB := submitTestProposal(t, store, "B")
	if _, err := Adjudicate(store, Verdict{Fingerprint: fpA, Decision: VerdictAccept, Rationale: "ok", DecidedBy: "engine"}); err != nil {
		t.Fatal(err)
	}
	cut := store.currentClock()
	if _, err := Adjudicate(store, Verdict{Fingerprint: fpB, Decision: VerdictReject, Rationale: "no", DecidedBy: "engine"}); err != nil {
		t.Fatal(err)
	}

	all, err := ListVerdicts(store, nowUnboundedAsOf)
	if err != nil || len(all) != 2 {
		t.Fatalf("应返回两条裁决: %d err=%v", len(all), err)
	}
	if all[0].Fingerprint > all[1].Fingerprint {
		t.Fatal("裁决列表应按 fingerprint 升序")
	}

	before, err := ListVerdicts(store, cut)
	if err != nil || len(before) != 1 || before[0].Fingerprint != fpA {
		t.Fatalf("as-of 上界应只看到第一条裁决: %+v err=%v", before, err)
	}
}

func writeTaskState(t *testing.T, store *fakeReadWriter, st taskplane.Status) {
	t.Helper()
	payload, err := jsonMarshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutVersioned(taskplane.StatusKey(st.Name), payload, "taskplane"); err != nil {
		t.Fatal(err)
	}
}

func TestBuildContextIncludesTasksProposalsAndVerdicts(t *testing.T) {
	store := newFakeReadWriter()
	writeTaskState(t, store, taskplane.Status{
		Name: "research:daily", Phase: taskplane.PhaseRunning, Owner: "worker-a",
		Attempt: 2, NextDueNanos: 100, LastError: "retry",
	})
	writeTaskState(t, store, taskplane.Status{Name: "research:weekly", Phase: taskplane.PhaseIdle, NextDueNanos: 900})

	fp := submitTestProposal(t, store, "候选假设")
	if _, err := Adjudicate(store, Verdict{Fingerprint: fp, Decision: VerdictReject, Rationale: "证据不足", DecidedBy: "engine"}); err != nil {
		t.Fatal(err)
	}

	b, err := BuildContext(store, ContextRequest{AsOfNanos: nowUnboundedAsOf}, repoContractsDir, repoRedlinesPath)
	if err != nil {
		t.Fatalf("BuildContext 失败: %v", err)
	}

	if len(b.TaskStates) != 2 || b.TaskStates[0].Name != "research:daily" || b.TaskStates[0].Phase != "running" {
		t.Fatalf("任务状态不正确: %+v", b.TaskStates)
	}
	if b.TaskStates[0].Attempt != 2 || b.TaskStates[0].Owner != "worker-a" || b.TaskStates[0].LastError != "retry" {
		t.Fatalf("任务字段丢失: %+v", b.TaskStates[0])
	}
	// 已裁决的提议不再出现在待裁决列表里。
	if len(b.OpenProposals) != 0 {
		t.Fatalf("已裁决提议不应出现在 open_proposals: %+v", b.OpenProposals)
	}
	if len(b.Verdicts) != 1 || b.Verdicts[0].Decision != VerdictReject || b.Verdicts[0].Rationale != "证据不足" {
		t.Fatalf("裁决列表不正确: %+v", b.Verdicts)
	}
}

func TestBuildContextKeepsUnaudicatedProposalOpen(t *testing.T) {
	store := newFakeReadWriter()
	fp := submitTestProposal(t, store, "待裁决假设")

	b, err := BuildContext(store, ContextRequest{AsOfNanos: nowUnboundedAsOf}, repoContractsDir, repoRedlinesPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.OpenProposals) != 1 || b.OpenProposals[0].Fingerprint != fp {
		t.Fatalf("未裁决提议应保持 open: %+v", b.OpenProposals)
	}
	if b.OpenProposals[0].SubmittedBy != "researcher-v1" || b.OpenProposals[0].Summary != "待裁决假设" {
		t.Fatalf("提议摘要字段不完整: %+v", b.OpenProposals[0])
	}
}

func TestBuildContextTaskStatesRespectAsOf(t *testing.T) {
	store := newFakeReadWriter()
	writeTaskState(t, store, taskplane.Status{Name: "research:daily", Phase: taskplane.PhaseIdle})
	cut := store.currentClock()
	writeTaskState(t, store, taskplane.Status{Name: "research:daily", Phase: taskplane.PhaseRunning, Attempt: 1})

	b, err := BuildContext(store, ContextRequest{AsOfNanos: cut}, repoContractsDir, repoRedlinesPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.TaskStates) != 1 || b.TaskStates[0].Phase != "idle" {
		t.Fatalf("as-of 上界应看到旧状态: %+v", b.TaskStates)
	}
}
