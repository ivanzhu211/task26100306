package burndown

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"
)

// 本文件用一份独立的从头重放实现校验 Ledger：
// 对随机生成的提交历史，每个观察版本、每个冲刺的报告都必须
// 与“只重放前 v 次提交”的朴素结果逐日一致。

// op 记录一次成功提交的原始参数。
type op struct {
	kind  int // 0=add 1=correct 2=retract
	fact  FactID
	story StoryID
	at    time.Time
	st    State
}

// refFact 是重放模型中的事实。
type refFact struct {
	story     StoryID
	at        time.Time
	st        State
	retracted bool
}

// replayReport 从头重放前 v 次提交，朴素计算冲刺报告。
func replayReport(ops []op, v Version, sprint SprintID, start time.Time) Report {
	facts := map[FactID]*refFact{}
	for i, o := range ops {
		if Version(i+1) > v {
			break
		}
		switch o.kind {
		case 0:
			facts[o.fact] = &refFact{story: o.story, at: o.at, st: o.st}
		case 1:
			facts[o.fact].st = o.st
		case 2:
			facts[o.fact].retracted = true
		}
	}
	byStory := map[StoryID][]*refFact{}
	for _, f := range facts {
		if !f.retracted {
			byStory[f.story] = append(byStory[f.story], f)
		}
	}
	rep := Report{Sprint: sprint, Version: v, Start: start}
	var scope [SprintDays]int
	for _, fs := range byStory {
		sort.Slice(fs, func(i, j int) bool { return fs[i].at.Before(fs[j].at) })
		stateAt := func(deadline time.Time, inclusive bool) (State, bool) {
			var cur *refFact
			for _, f := range fs {
				if f.at.Before(deadline) || (inclusive && f.at.Equal(deadline)) {
					cur = f
				} else {
					break
				}
			}
			if cur == nil {
				return State{}, false
			}
			return cur.st, true
		}
		if st, ok := stateAt(start, true); ok && st.Sprint == sprint {
			rep.OpeningCommitment += st.Points
		}
		for d := 0; d < SprintDays; d++ {
			end := start.AddDate(0, 0, d+1)
			if st, ok := stateAt(end, false); ok && st.Sprint == sprint {
				scope[d] += st.Points
				if !st.Done {
					rep.Days[d].Remaining += st.Points
				}
			}
		}
	}
	for d := 0; d < SprintDays; d++ {
		rep.Days[d].NetScopeChange = scope[d] - rep.OpeningCommitment
	}
	return rep
}

func TestRandomHistoryMatchesReplay(t *testing.T) {
	rng := rand.New(rand.NewSource(20261002))
	l := NewLedger()
	sprints := []SprintID{"S1", "S2", "S3"}
	starts := map[SprintID]time.Time{}
	for i, s := range sprints {
		start := day0.AddDate(0, 0, SprintDays*(i-1)) // 一个冲刺跨在故事时间窗中段
		starts[s] = start
		mustDefine(t, l, s, start)
	}

	stories := []StoryID{"A", "B", "C", "D", "E", "F", "G", "H"}
	usedBT := map[StoryID]map[int64]bool{}
	for _, s := range stories {
		usedBT[s] = map[int64]bool{}
	}
	randBT := func(s StoryID) time.Time {
		for {
			// 覆盖冲刺开始前、日界线零点、冲刺结束时刻等边界。
			var at time.Time
			switch rng.Intn(5) {
			case 0:
				at = day0.AddDate(0, 0, rng.Intn(40)-10) // 整点日界线
			case 1:
				at = day0.AddDate(0, 0, rng.Intn(40)-10).Add(time.Duration(rng.Intn(24)) * time.Hour)
			default:
				at = day0.AddDate(0, 0, rng.Intn(40)-10).
					Add(time.Duration(rng.Intn(24*60)) * time.Minute)
			}
			if !usedBT[s][at.Unix()] {
				usedBT[s][at.Unix()] = true
				return at
			}
		}
	}
	randState := func() State {
		st := State{Points: rng.Intn(9), Done: rng.Intn(3) == 0}
		if rng.Intn(4) != 0 {
			st.Sprint = sprints[rng.Intn(len(sprints))]
		}
		return st
	}

	var ops []op
	var live []FactID // 未撤销的事实
	for i := 0; i < 400; i++ {
		roll := rng.Intn(10)
		switch {
		case roll < 5 || len(live) == 0:
			f := FactID(fmt.Sprintf("f%d", i))
			s := stories[rng.Intn(len(stories))]
			o := op{kind: 0, fact: f, story: s, at: randBT(s), st: randState()}
			if _, err := l.AddFact(o.fact, o.story, o.at, o.st); err != nil {
				t.Fatalf("op %d add: %v", i, err)
			}
			ops = append(ops, o)
			live = append(live, f)
		case roll < 8:
			idx := rng.Intn(len(live))
			o := op{kind: 1, fact: live[idx], st: randState()}
			if _, err := l.CorrectFact(o.fact, o.st); err != nil {
				t.Fatalf("op %d correct: %v", i, err)
			}
			ops = append(ops, o)
		default:
			idx := rng.Intn(len(live))
			o := op{kind: 2, fact: live[idx]}
			if _, err := l.RetractFact(o.fact); err != nil {
				t.Fatalf("op %d retract: %v", i, err)
			}
			ops = append(ops, o)
			live = append(live[:idx], live[idx+1:]...)
		}
	}

	// 每个观察版本、每个冲刺都与从头重放逐日一致。
	for v := Version(0); v <= l.Version(); v++ {
		for _, s := range sprints {
			got := mustReport(t, l, s, v)
			want := replayReport(ops, v, s, starts[s])
			if got != want {
				t.Fatalf("sprint %s version %d mismatch\ngot:  %+v\nwant: %+v", s, v, got, want)
			}
		}
	}
}
