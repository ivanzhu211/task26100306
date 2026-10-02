package burndown

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // UTC 零点

func at(d int) time.Time { return t0.Add(time.Duration(d) * dayDur) }

func mustDefine(t *testing.T, l *Ledger, id int64) {
	t.Helper()
	if err := l.DefineSprint(id, t0); err != nil {
		t.Fatalf("DefineSprint(%d): %v", id, err)
	}
}

func mustAdd(t *testing.T, l *Ledger, f Fact) uint64 {
	t.Helper()
	v, err := l.SubmitAdd(f)
	if err != nil {
		t.Fatalf("SubmitAdd(%+v): %v", f, err)
	}
	return v
}

func mustReport(t *testing.T, l *Ledger, sprintID int64, v uint64) Report {
	t.Helper()
	r, err := l.Report(sprintID, v)
	if err != nil {
		t.Fatalf("Report(%d, %d): %v", sprintID, v, err)
	}
	return r
}

// 晚到事实改变开场承诺，但旧版本报告不变。
func TestLateFactChangesOpeningButOldVersionStable(t *testing.T) {
	l := New()
	mustDefine(t, l, 1)

	v1 := mustAdd(t, l, Fact{FactID: "a1", StoryID: "s1", Time: at(2),
		State: State{SprintID: 1, Points: 5}})

	before := mustReport(t, l, 1, v1)
	if before.Opening != 0 {
		t.Fatalf("v1 opening = %d, want 0", before.Opening)
	}
	// 事实在 T0+2d 零点生效，归第 3 天。
	if before.Days[1].Remaining != 0 || before.Days[2].Remaining != 5 {
		t.Fatalf("v1 days = %+v", before.Days)
	}

	// 补录一条业务时刻早于冲刺开始的事实（已完成）。
	v2 := mustAdd(t, l, Fact{FactID: "b1", StoryID: "s2", Time: at(-1),
		State: State{SprintID: 1, Points: 8, Done: true}})

	again := mustReport(t, l, 1, v1)
	if again != before {
		t.Fatalf("v1 report changed after later submission:\nold %+v\nnew %+v", before, again)
	}
	after := mustReport(t, l, 1, v2)
	if after.Opening != 8 {
		t.Fatalf("v2 opening = %d, want 8（已完成故事计入开场承诺）", after.Opening)
	}
	// 已完成：剩余恒 0；范围 = 8 + 第3天起 s1 的 5 点。
	if after.Days[0].Remaining != 0 || after.Days[0].ScopeNetChange != 0 {
		t.Fatalf("v2 day1 = %+v", after.Days[0])
	}
	if after.Days[2].Remaining != 5 || after.Days[2].ScopeNetChange != 5 {
		t.Fatalf("v2 day3 = %+v", after.Days[2])
	}
}

// 撤销一次跨冲刺移出：前一状态延续，两个冲刺同时修正。
func TestRevokeCrossSprintMove(t *testing.T) {
	l := New()
	mustDefine(t, l, 1)
	mustDefine(t, l, 2)

	v1 := mustAdd(t, l, Fact{FactID: "a1", StoryID: "s1", Time: at(1),
		State: State{SprintID: 1, Points: 5}})
	v2 := mustAdd(t, l, Fact{FactID: "a2", StoryID: "s1", Time: at(3),
		State: State{SprintID: 2, Points: 5}})

	r1v2 := mustReport(t, l, 1, v2)
	if r1v2.Days[2].Remaining != 5 || r1v2.Days[3].Remaining != 0 {
		t.Fatalf("sprint1 v2 days = %+v", r1v2.Days)
	}
	r2v2 := mustReport(t, l, 2, v2)
	if r2v2.Days[2].Remaining != 0 || r2v2.Days[3].Remaining != 5 {
		t.Fatalf("sprint2 v2 days = %+v", r2v2.Days)
	}

	v3, err := l.SubmitRevoke("a2")
	if err != nil {
		t.Fatalf("SubmitRevoke: %v", err)
	}
	// 撤销后冲刺1中 s1 的状态从 T0+1d 延续到冲刺结束。
	r1v3 := mustReport(t, l, 1, v3)
	for d := 0; d < SprintDays; d++ {
		want := 0
		if d >= 1 {
			want = 5
		}
		if r1v3.Days[d].Remaining != want {
			t.Fatalf("sprint1 v3 day%d remaining = %d, want %d", d+1, r1v3.Days[d].Remaining, want)
		}
	}
	r2v3 := mustReport(t, l, 2, v3)
	for d := 0; d < SprintDays; d++ {
		if r2v3.Days[d].Remaining != 0 || r2v3.Days[d].ScopeNetChange != 0 {
			t.Fatalf("sprint2 v3 day%d = %+v, want zero", d+1, r2v3.Days[d])
		}
	}
	// 旧版本不受影响。
	if got := mustReport(t, l, 1, v2); got != r1v2 {
		t.Fatalf("sprint1 v2 changed after revoke")
	}
	if got := mustReport(t, l, 2, v2); got != r2v2 {
		t.Fatalf("sprint2 v2 changed after revoke")
	}
	_ = v1
}

// 完成、重新打开、估点变化发生在冲刺开始、日界线、结束时刻。
func TestBoundaryMoments(t *testing.T) {
	l := New()
	mustDefine(t, l, 1)

	// s1：冲刺开始时刻完成。
	mustAdd(t, l, Fact{FactID: "f1", StoryID: "s1", Time: at(0),
		State: State{SprintID: 1, Points: 3, Done: true}})
	// s2：日界线（第5天结束零点）完成。
	mustAdd(t, l, Fact{FactID: "f2", StoryID: "s2", Time: at(0),
		State: State{SprintID: 1, Points: 5}})
	mustAdd(t, l, Fact{FactID: "f3", StoryID: "s2", Time: at(5),
		State: State{SprintID: 1, Points: 5, Done: true}})
	// s3：结束时刻才入冲刺，永不计入。
	mustAdd(t, l, Fact{FactID: "f4", StoryID: "s3", Time: at(14),
		State: State{SprintID: 1, Points: 7}})
	// s4：开始时刻估点 4，日界线重新打开后又估点变化。
	mustAdd(t, l, Fact{FactID: "f5", StoryID: "s4", Time: at(0),
		State: State{SprintID: 1, Points: 4, Done: true}})
	mustAdd(t, l, Fact{FactID: "f6", StoryID: "s4", Time: at(6),
		State: State{SprintID: 1, Points: 9}})

	r := mustReport(t, l, 1, l.Version())
	// 开场承诺 = 3(s1) + 5(s2) + 4(s4)，s3 不计。
	if r.Opening != 12 {
		t.Fatalf("opening = %d, want 12", r.Opening)
	}
	// 剩余：第1-5天只有 s2 的 5 点；第5天日界线的完成归第6天。
	for d := 0; d < 5; d++ {
		if r.Days[d].Remaining != 5 {
			t.Fatalf("day%d remaining = %d, want 5", d+1, r.Days[d].Remaining)
		}
	}
	// 第6天：s2 完成；s4 的重新打开恰在第6天结束零点，归第7天。
	if r.Days[5].Remaining != 0 {
		t.Fatalf("day6 remaining = %d, want 0", r.Days[5].Remaining)
	}
	// 第7天起：s4 重新打开且估点 9。
	for d := 6; d < SprintDays; d++ {
		if r.Days[d].Remaining != 9 {
			t.Fatalf("day%d remaining = %d, want 9", d+1, r.Days[d].Remaining)
		}
	}
	// 范围净变化：第6天总范围仍为 12（s4 自始完成，未离开过范围），净 0；
	// 第7天起 s4 以 9 点重新打开（12-4+9=17，净 +5）。s3 永不计入。
	if r.Days[5].ScopeNetChange != 0 {
		t.Fatalf("day6 scope net = %d, want 0", r.Days[5].ScopeNetChange)
	}
	if r.Days[6].ScopeNetChange != 5 {
		t.Fatalf("day7 scope net = %d, want 5", r.Days[6].ScopeNetChange)
	}
}

// 更正估点：旧版本不变，新版本按更正后状态。
func TestCorrectPoints(t *testing.T) {
	l := New()
	mustDefine(t, l, 1)
	v1 := mustAdd(t, l, Fact{FactID: "a1", StoryID: "s1", Time: at(0),
		State: State{SprintID: 1, Points: 5}})
	v2, err := l.SubmitCorrect("a1", State{SprintID: 1, Points: 13})
	if err != nil {
		t.Fatalf("SubmitCorrect: %v", err)
	}
	if got := mustReport(t, l, 1, v1); got.Opening != 5 {
		t.Fatalf("v1 opening = %d, want 5", got.Opening)
	}
	if got := mustReport(t, l, 1, v2); got.Opening != 13 {
		t.Fatalf("v2 opening = %d, want 13", got.Opening)
	}
}

// 非法输入。
func TestValidation(t *testing.T) {
	l := New()
	if err := l.DefineSprint(1, t0.Add(time.Hour)); err == nil {
		t.Fatal("non-midnight start accepted")
	}
	if err := l.DefineSprint(1, t0); err != nil {
		t.Fatal(err)
	}
	if err := l.DefineSprint(1, t0); err == nil {
		t.Fatal("duplicate sprint accepted")
	}
	if _, err := l.SubmitAdd(Fact{FactID: "x", StoryID: "s", Time: at(0),
		State: State{SprintID: 99, Points: 1}}); err == nil {
		t.Fatal("undefined sprint accepted")
	}
	mustAdd(t, l, Fact{FactID: "a", StoryID: "s", Time: at(0), State: State{SprintID: 1, Points: 1}})
	if _, err := l.SubmitAdd(Fact{FactID: "a", StoryID: "s", Time: at(1), State: State{}}); err == nil {
		t.Fatal("duplicate fact id accepted")
	}
	if _, err := l.SubmitAdd(Fact{FactID: "b", StoryID: "s", Time: at(0), State: State{}}); err == nil {
		t.Fatal("duplicate business time accepted")
	}
	if _, err := l.SubmitCorrect("nope", State{}); err == nil {
		t.Fatal("correct unknown fact accepted")
	}
	if _, err := l.SubmitRevoke("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.SubmitRevoke("a"); err == nil {
		t.Fatal("double revoke accepted")
	}
	if _, err := l.SubmitCorrect("a", State{}); err == nil {
		t.Fatal("correct revoked fact accepted")
	}
	if _, err := l.Report(1, l.Version()+1); err == nil {
		t.Fatal("future version accepted")
	}
}
