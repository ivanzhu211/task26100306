package burndown

import (
	"testing"
	"time"
)

var day0 = time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // 周一

func mustDefine(t *testing.T, l *Ledger, id SprintID, start time.Time) {
	t.Helper()
	if err := l.DefineSprint(id, start); err != nil {
		t.Fatalf("DefineSprint(%s): %v", id, err)
	}
}

func mustAdd(t *testing.T, l *Ledger, f FactID, s StoryID, at time.Time, st State) Version {
	t.Helper()
	v, err := l.AddFact(f, s, at, st)
	if err != nil {
		t.Fatalf("AddFact(%s): %v", f, err)
	}
	return v
}

func mustReport(t *testing.T, l *Ledger, s SprintID, v Version) Report {
	t.Helper()
	r, err := l.Report(s, v)
	if err != nil {
		t.Fatalf("Report(%s, %d): %v", s, v, err)
	}
	return r
}

// 冲刺开始时刻属于本冲刺的故事计入开场承诺，含已完成故事。
func TestOpeningCommitmentBasics(t *testing.T) {
	l := NewLedger()
	mustDefine(t, l, "S1", day0)
	before := day0.Add(-time.Hour)
	mustAdd(t, l, "f1", "A", before, State{Sprint: "S1", Points: 5})
	mustAdd(t, l, "f2", "B", before, State{Sprint: "S1", Points: 3, Done: true})
	mustAdd(t, l, "f3", "C", before, State{Sprint: "", Points: 8}) // 未入冲刺

	r := mustReport(t, l, "S1", l.Version())
	if r.OpeningCommitment != 8 {
		t.Fatalf("opening = %d, want 8", r.OpeningCommitment)
	}
	if r.Days[0].Remaining != 5 {
		t.Fatalf("day1 remaining = %d, want 5", r.Days[0].Remaining)
	}
	if r.Days[0].NetScopeChange != 0 {
		t.Fatalf("day1 net change = %d, want 0", r.Days[0].NetScopeChange)
	}
}

// 晚到事实（业务时刻早于冲刺开始）改变新版本的开场承诺，旧版本报告不变。
func TestLateFactChangesOpeningButOldVersionStable(t *testing.T) {
	l := NewLedger()
	mustDefine(t, l, "S1", day0)
	v1 := mustAdd(t, l, "f1", "A", day0.Add(-time.Hour), State{Sprint: "S1", Points: 5})
	before := mustReport(t, l, "S1", v1)
	if before.OpeningCommitment != 5 {
		t.Fatalf("opening@v1 = %d, want 5", before.OpeningCommitment)
	}

	// 晚到：业务时刻在冲刺开始之前，把 B 补录进 S1。
	v2 := mustAdd(t, l, "f2", "B", day0.Add(-2*time.Hour), State{Sprint: "S1", Points: 7})
	after := mustReport(t, l, "S1", v2)
	if after.OpeningCommitment != 12 {
		t.Fatalf("opening@v2 = %d, want 12", after.OpeningCommitment)
	}
	// 旧版本报告必须不变。
	again := mustReport(t, l, "S1", v1)
	if again != before {
		t.Fatalf("report@v1 changed: %+v -> %+v", before, again)
	}
}

// 撤销一次跨冲刺移出：前一状态延续，两个冲刺同时修正。
func TestRetractCrossSprintMove(t *testing.T) {
	l := NewLedger()
	mustDefine(t, l, "S1", day0)
	day14 := day0.AddDate(0, 0, SprintDays)
	mustDefine(t, l, "S2", day14)

	mustAdd(t, l, "f1", "A", day0.Add(-time.Hour), State{Sprint: "S1", Points: 5})
	// 第 3 天移出 S1、进入 S2。
	moveAt := day0.AddDate(0, 0, 2).Add(time.Hour)
	mustAdd(t, l, "f2", "A", moveAt, State{Sprint: "S2", Points: 5})
	vMoved := l.Version()

	r1 := mustReport(t, l, "S1", vMoved)
	if r1.Days[1].Remaining != 5 || r1.Days[2].Remaining != 0 {
		t.Fatalf("S1 remaining around move = %d,%d, want 5,0",
			r1.Days[1].Remaining, r1.Days[2].Remaining)
	}
	if r1.Days[2].NetScopeChange != -5 {
		t.Fatalf("S1 net change after move = %d, want -5", r1.Days[2].NetScopeChange)
	}
	r2 := mustReport(t, l, "S2", vMoved)
	if r2.OpeningCommitment != 5 {
		t.Fatalf("S2 opening = %d, want 5", r2.OpeningCommitment)
	}

	// 撤销移出：A 重新属于 S1 全程，S2 不再包含 A。
	vRetracted, err := l.RetractFact("f2")
	if err != nil {
		t.Fatalf("RetractFact: %v", err)
	}
	r1 = mustReport(t, l, "S1", vRetracted)
	for d := 0; d < SprintDays; d++ {
		if r1.Days[d].Remaining != 5 {
			t.Fatalf("S1 day%d remaining = %d, want 5", d+1, r1.Days[d].Remaining)
		}
		if r1.Days[d].NetScopeChange != 0 {
			t.Fatalf("S1 day%d net change = %d, want 0", d+1, r1.Days[d].NetScopeChange)
		}
	}
	r2 = mustReport(t, l, "S2", vRetracted)
	if r2.OpeningCommitment != 0 {
		t.Fatalf("S2 opening after retract = %d, want 0", r2.OpeningCommitment)
	}
	// 撤销前的版本报告仍反映移出。
	r1old := mustReport(t, l, "S1", vMoved)
	if r1old.Days[2].Remaining != 0 {
		t.Fatalf("S1@vMoved day3 remaining = %d, want 0", r1old.Days[2].Remaining)
	}
}

// 完成、重新打开、估点变化发生在冲刺开始、日界线、结束时刻。
func TestBoundaryInstants(t *testing.T) {
	l := NewLedger()
	mustDefine(t, l, "S1", day0)

	// A：恰在冲刺开始时刻完成 —— 计入开场承诺，日终剩余为 0。
	mustAdd(t, l, "f1", "A", day0.Add(-time.Hour), State{Sprint: "S1", Points: 3})
	mustAdd(t, l, "f2", "A", day0, State{Sprint: "S1", Points: 3, Done: true})

	// B：恰在第 2 天零点（日界线）完成 —— 归次日，第 1 天日终仍未完成。
	mustAdd(t, l, "f3", "B", day0.Add(-time.Hour), State{Sprint: "S1", Points: 4})
	mustAdd(t, l, "f4", "B", day0.AddDate(0, 0, 1), State{Sprint: "S1", Points: 4, Done: true})

	// C：恰在冲刺结束时刻完成 —— 对本次冲刺 14 天无影响。
	mustAdd(t, l, "f5", "C", day0.Add(-time.Hour), State{Sprint: "S1", Points: 2})
	mustAdd(t, l, "f6", "C", day0.AddDate(0, 0, SprintDays), State{Sprint: "S1", Points: 2, Done: true})

	// D：第 1 天日界线估点 5->8，第 3 天重新打开后又完成。
	mustAdd(t, l, "f7", "D", day0.Add(-time.Hour), State{Sprint: "S1", Points: 5, Done: true})
	mustAdd(t, l, "f8", "D", day0.AddDate(0, 0, 1), State{Sprint: "S1", Points: 8, Done: true})
	mustAdd(t, l, "f9", "D", day0.AddDate(0, 0, 2).Add(6*time.Hour), State{Sprint: "S1", Points: 8})
	mustAdd(t, l, "f10", "D", day0.AddDate(0, 0, 3), State{Sprint: "S1", Points: 8, Done: true})

	r := mustReport(t, l, "S1", l.Version())
	if r.OpeningCommitment != 14 {
		t.Fatalf("opening = %d, want 14 (3+4+2+5)", r.OpeningCommitment)
	}
	// 第 1 天日终：A 完成、B 未完成、C 未完成、D 完成(5)。
	if r.Days[0].Remaining != 6 {
		t.Fatalf("day1 remaining = %d, want 6 (B4+C2)", r.Days[0].Remaining)
	}
	if r.Days[0].NetScopeChange != 0 {
		t.Fatalf("day1 net = %d, want 0", r.Days[0].NetScopeChange)
	}
	// 第 2 天日终：B 完成；D 估点变为 8，范围净变化 +3。
	if r.Days[1].Remaining != 2 {
		t.Fatalf("day2 remaining = %d, want 2 (C2)", r.Days[1].Remaining)
	}
	if r.Days[1].NetScopeChange != 3 {
		t.Fatalf("day2 net = %d, want 3", r.Days[1].NetScopeChange)
	}
	// 第 3 天日终：D 重新打开，剩余含 D8。
	if r.Days[2].Remaining != 10 {
		t.Fatalf("day3 remaining = %d, want 10 (C2+D8)", r.Days[2].Remaining)
	}
	// 第 4 天起：D 再次完成；C 的完成恰在结束时刻，不影响本冲刺。
	for d := 3; d < SprintDays; d++ {
		if r.Days[d].Remaining != 2 {
			t.Fatalf("day%d remaining = %d, want 2 (C2)", d+1, r.Days[d].Remaining)
		}
		if r.Days[d].NetScopeChange != 3 {
			t.Fatalf("day%d net = %d, want 3", d+1, r.Days[d].NetScopeChange)
		}
	}
}

// 更正只替换状态；后续完整状态不被自动改写。
func TestCorrectionDoesNotRewriteLaterFacts(t *testing.T) {
	l := NewLedger()
	mustDefine(t, l, "S1", day0)
	mustAdd(t, l, "f1", "A", day0.Add(-time.Hour), State{Sprint: "S1", Points: 5})
	mustAdd(t, l, "f2", "A", day0.AddDate(0, 0, 1), State{Sprint: "S1", Points: 5, Done: true})
	v0 := l.Version()

	// 把第一条更正为 8 点：第 1 天日终仍是后续事实的 5 点完成态。
	if _, err := l.CorrectFact("f1", State{Sprint: "S1", Points: 8}); err != nil {
		t.Fatalf("CorrectFact: %v", err)
	}
	r := mustReport(t, l, "S1", l.Version())
	if r.OpeningCommitment != 8 {
		t.Fatalf("opening = %d, want 8", r.OpeningCommitment)
	}
	// f2 恰在第 2 天零点生效，归次日：第 1 天日终仍是更正后的 8 点未完成。
	if r.Days[0].Remaining != 8 || r.Days[0].NetScopeChange != 0 {
		t.Fatalf("day1 = %+v, want remaining 8 net 0", r.Days[0])
	}
	// 第 2 天日终起为后续事实的 5 点完成态，不被更正改写。
	if r.Days[1].Remaining != 0 || r.Days[1].NetScopeChange != -3 {
		t.Fatalf("day2 = %+v, want remaining 0 net -3", r.Days[1])
	}
	old := mustReport(t, l, "S1", v0)
	if old.OpeningCommitment != 5 {
		t.Fatalf("opening@v0 = %d, want 5", old.OpeningCommitment)
	}
}

// 提交校验：重复事实、重复业务时刻、撤销后再更正、版本越界等。
func TestSubmissionValidation(t *testing.T) {
	l := NewLedger()
	mustDefine(t, l, "S1", day0)
	if err := l.DefineSprint("S1", day0); err != ErrSprintExists {
		t.Fatalf("redefine err = %v", err)
	}
	if err := l.DefineSprint("S2", day0.Add(time.Hour)); err != ErrInvalidSprintStart {
		t.Fatalf("bad start err = %v", err)
	}
	if _, err := l.AddFact("f1", "A", day0, State{Sprint: "S9", Points: 1}); err != ErrSprintNotDefined {
		t.Fatalf("undefined sprint err = %v", err)
	}
	if _, err := l.AddFact("f1", "A", day0, State{Sprint: "S1", Points: -1}); err != ErrNegativePoints {
		t.Fatalf("negative points err = %v", err)
	}
	mustAdd(t, l, "f1", "A", day0, State{Sprint: "S1", Points: 1})
	if _, err := l.AddFact("f1", "A", day0, State{Sprint: "S1", Points: 1}); err != ErrFactExists {
		t.Fatalf("dup fact err = %v", err)
	}
	if _, err := l.AddFact("f2", "A", day0, State{Sprint: "S1", Points: 1}); err != ErrDuplicateBusinessTime {
		t.Fatalf("dup bt err = %v", err)
	}
	if _, err := l.CorrectFact("nope", State{}); err != ErrFactNotFound {
		t.Fatalf("correct missing err = %v", err)
	}
	if _, err := l.RetractFact("f1"); err != nil {
		t.Fatalf("retract: %v", err)
	}
	if _, err := l.RetractFact("f1"); err != ErrFactRetracted {
		t.Fatalf("re-retract err = %v", err)
	}
	if _, err := l.CorrectFact("f1", State{}); err != ErrFactRetracted {
		t.Fatalf("correct retracted err = %v", err)
	}
	if _, err := l.Report("S1", l.Version()+1); err != ErrVersionOutOfRange {
		t.Fatalf("future version err = %v", err)
	}
	if _, err := l.Report("S9", 0); err != ErrSprintNotDefined {
		t.Fatalf("report undefined sprint err = %v", err)
	}
}
