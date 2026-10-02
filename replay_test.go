package burndown

import (
	"math/rand"
	"sort"
	"strconv"
	"testing"
	"time"
)

// 以下是与 Ledger 实现完全独立的朴素参照：保存全部提交日志，
// 每次查询从头重放，用于逐版本逐日比对。

type submission struct {
	kind   string // "add" | "correct" | "revoke"
	fact   Fact
	factID string
	state  State
}

type refLedger struct {
	starts map[int64]time.Time
	subs   []submission
}

// stateAt 返回故事在业务时刻 t 的状态。
// strict 为 true 时取 bt < t 的最后一条（日终），否则取 bt <= t（开场）。
func stateAt(facts []Fact, t time.Time, strict bool) (State, bool) {
	best := -1
	for i, f := range facts {
		if f.Time.Before(t) || (!strict && f.Time.Equal(t)) {
			best = i
		}
	}
	if best < 0 {
		return State{}, false
	}
	return facts[best].State, true
}

func (r *refLedger) report(sprintID int64, version uint64) Report {
	rep := Report{SprintID: sprintID, Version: version}
	t0 := r.starts[sprintID]
	// 重放前 version 次提交，得到各故事有效事实（含当时最新状态）。
	live := map[string]Fact{}
	revoked := map[string]bool{}
	storyOf := map[string]string{}
	for i := uint64(0); i < version && i < uint64(len(r.subs)); i++ {
		s := r.subs[i]
		switch s.kind {
		case "add":
			live[s.fact.FactID] = s.fact
			storyOf[s.fact.FactID] = s.fact.StoryID
		case "correct":
			f := live[s.factID]
			f.State = s.state
			live[s.factID] = f
		case "revoke":
			revoked[s.factID] = true
		}
	}
	byStory := map[string][]Fact{}
	for id, f := range live {
		if revoked[id] {
			continue
		}
		sid := storyOf[id]
		byStory[sid] = append(byStory[sid], f)
	}
	for _, facts := range byStory {
		sort.Slice(facts, func(i, j int) bool { return facts[i].Time.Before(facts[j].Time) })
		if st, ok := stateAt(facts, t0, false); ok && st.SprintID == sprintID {
			rep.Opening += st.Points
		}
		for d := 1; d <= SprintDays; d++ {
			t := t0.Add(time.Duration(d) * dayDur)
			if st, ok := stateAt(facts, t, true); ok && st.SprintID == sprintID {
				rep.Days[d-1].ScopeNetChange += st.Points
				if !st.Done {
					rep.Days[d-1].Remaining += st.Points
				}
			}
		}
	}
	for d := 0; d < SprintDays; d++ {
		rep.Days[d].ScopeNetChange -= rep.Opening
	}
	return rep
}

// 小规模随机历史：每个观察版本、每个冲刺都与从头重放逐日一致。
func TestRandomHistoryMatchesReplay(t *testing.T) {
	const (
		sprints     = 3
		stories     = 8
		submissions = 300
	)
	rng := rand.New(rand.NewSource(20261002))
	l := New()
	ref := &refLedger{starts: map[int64]time.Time{}}
	for s := int64(1); s <= sprints; s++ {
		mustDefine(t, l, s)
		ref.starts[s] = t0
	}

	randState := func() State {
		sp := int64(rng.Intn(sprints + 1)) // 0 = NoSprint, 1..sprints
		if sp == 0 {
			sp = NoSprint
		}
		return State{SprintID: sp, Points: rng.Intn(11), Done: rng.Intn(3) == 0}
	}
	// 业务时刻集中在冲刺开始、日界线、结束时刻附近，且按故事去重。
	usedTime := map[string]map[int]bool{}
	randTime := func(story string) time.Time {
		if usedTime[story] == nil {
			usedTime[story] = map[int]bool{}
		}
		for {
			h := rng.Intn(41)*24 - 5*24 // [-5d, 35d] 的整点零点
			if !usedTime[story][h] {
				usedTime[story][h] = true
				return t0.Add(time.Duration(h) * time.Hour)
			}
		}
	}

	storyIDs := []string{}
	for i := 0; i < stories; i++ {
		storyIDs = append(storyIDs, string(rune('A'+i)))
	}
	liveFacts := []string{}
	factN := 0
	for i := 0; i < submissions; i++ {
		op := rng.Intn(10)
		switch {
		case op < 5 || len(liveFacts) == 0: // 新增
			f := Fact{
				FactID:  "f" + strconv.Itoa(factN),
				StoryID: storyIDs[rng.Intn(stories)],
				State:   randState(),
			}
			factN++
			f.Time = randTime(f.StoryID)
			v, err := l.SubmitAdd(f)
			if err != nil {
				t.Fatalf("submit %d add %+v: %v", i, f, err)
			}
			if v != uint64(i+1) {
				t.Fatalf("version = %d, want %d", v, i+1)
			}
			ref.subs = append(ref.subs, submission{kind: "add", fact: f})
			liveFacts = append(liveFacts, f.FactID)
		case op < 8: // 更正
			idx := rng.Intn(len(liveFacts))
			id := liveFacts[idx]
			st := randState()
			if _, err := l.SubmitCorrect(id, st); err != nil {
				t.Fatalf("submit %d correct %s: %v", i, id, err)
			}
			ref.subs = append(ref.subs, submission{kind: "correct", factID: id, state: st})
		default: // 撤销
			idx := rng.Intn(len(liveFacts))
			id := liveFacts[idx]
			if _, err := l.SubmitRevoke(id); err != nil {
				t.Fatalf("submit %d revoke %s: %v", i, id, err)
			}
			ref.subs = append(ref.subs, submission{kind: "revoke", factID: id})
			liveFacts = append(liveFacts[:idx], liveFacts[idx+1:]...)
		}
	}

	// 每个观察版本 × 每个冲刺，逐日比对。
	for v := uint64(0); v <= l.Version(); v++ {
		for s := int64(1); s <= sprints; s++ {
			got, err := l.Report(s, v)
			if err != nil {
				t.Fatalf("Report(%d, %d): %v", s, v, err)
			}
			want := ref.report(s, v)
			if got != want {
				t.Fatalf("sprint %d version %d mismatch:\n got %+v\nwant %+v", s, v, got, want)
			}
		}
	}
}
