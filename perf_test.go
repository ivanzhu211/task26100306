package burndown

import (
	"fmt"
	"math/rand"
	"runtime"
	"strconv"
	"testing"
	"time"
)

// 规模性能验证：5万故事、500冲刺、20万事实 + 5万更正/撤销，
// 其中一个故事集中2万条事实；随后连续查询5000份覆盖不同版本与冲刺的报告。
// 可重复：固定随机种子。运行：go test -run TestPerfScale -v
func TestPerfScale(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	const (
		nSprints   = 500
		nStories   = 50000
		nFacts     = 200000
		nFixups    = 50000
		nHeavy     = 20000 // 单故事集中事实数
		nReports   = 5000
		submitLim  = 60 * time.Second
		reportLim  = 3 * time.Second
		memoryLim  = 512 << 20 // 512 MiB
		hoursRange = (nSprints*14 + 60) * 24
	)
	rng := rand.New(rand.NewSource(42))
	l := New()
	base := t0
	for s := int64(0); s < nSprints; s++ {
		if err := l.DefineSprint(s, base.Add(time.Duration(s)*14*dayDur)); err != nil {
			t.Fatal(err)
		}
	}

	used := make(map[string]map[int64]bool, 1024)
	randTimeFor := func(story string) time.Time {
		m := used[story]
		if m == nil {
			m = map[int64]bool{}
			used[story] = m
		}
		for {
			h := rng.Int63n(hoursRange) - 30*24
			if !m[h] {
				m[h] = true
				return base.Add(time.Duration(h) * time.Hour)
			}
		}
	}
	randState := func() State {
		sp := int64(rng.Intn(nSprints + 1))
		if sp == nSprints {
			sp = NoSprint
		}
		return State{SprintID: sp, Points: rng.Intn(14), Done: rng.Intn(4) == 0}
	}

	live := make([]string, 0, nFacts)
	start := time.Now()
	// 20万条新增：前 nHeavy 条集中在故事 "heavy"。
	for i := 0; i < nFacts; i++ {
		story := "story-" + strconv.Itoa(rng.Intn(nStories))
		if i < nHeavy {
			story = "heavy"
		}
		id := "fact-" + strconv.Itoa(i)
		f := Fact{FactID: id, StoryID: story, Time: randTimeFor(story), State: randState()}
		if _, err := l.SubmitAdd(f); err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
		live = append(live, id)
	}
	// 5万次更正/撤销。
	for i := 0; i < nFixups; i++ {
		idx := rng.Intn(len(live))
		id := live[idx]
		if rng.Intn(2) == 0 {
			if _, err := l.SubmitCorrect(id, randState()); err != nil {
				t.Fatalf("correct %d: %v", i, err)
			}
		} else {
			if _, err := l.SubmitRevoke(id); err != nil {
				t.Fatalf("revoke %d: %v", i, err)
			}
			live[idx] = live[len(live)-1]
			live = live[:len(live)-1]
		}
	}
	submitDur := time.Since(start)

	var mem runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&mem)

	// 5000份报告：随机覆盖不同观察版本与冲刺（含早期版本）。
	finalV := l.Version()
	start = time.Now()
	var checksum int
	for i := 0; i < nReports; i++ {
		v := uint64(rng.Int63n(int64(finalV) + 1))
		r, err := l.Report(int64(rng.Intn(nSprints)), v)
		if err != nil {
			t.Fatalf("report %d: %v", i, err)
		}
		checksum += r.Opening + r.Days[0].Remaining + r.Days[SprintDays-1].ScopeNetChange
	}
	reportDur := time.Since(start)

	t.Logf("submissions: %d in %v (limit %v)", nFacts+nFixups, submitDur, submitLim)
	t.Logf("reports:     %d in %v (limit %v), checksum=%d", nReports, reportDur, reportLim, checksum)
	t.Logf("memory:      HeapInuse=%.1f MiB (limit 512 MiB)", float64(mem.HeapInuse)/float64(1<<20))
	if submitDur > submitLim {
		t.Fatalf("submissions too slow: %v > %v", submitDur, submitLim)
	}
	if reportDur > reportLim {
		t.Fatalf("reports too slow: %v > %v", reportDur, reportLim)
	}
	if mem.HeapInuse > memoryLim {
		t.Fatalf("memory too high: %d > %d", mem.HeapInuse, memoryLim)
	}
	fmt.Printf("perf ok: submit=%v report=%v heap=%.1fMiB\n",
		submitDur.Round(time.Millisecond), reportDur.Round(time.Millisecond),
		float64(mem.HeapInuse)/float64(1<<20))
}
