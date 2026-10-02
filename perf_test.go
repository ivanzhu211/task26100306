package burndown

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"
)

// 规模验收：5 万故事、500 冲刺、20 万原始事实 + 5 万次更正/撤销，
// 提交总耗时 < 60s、堆内存 < 512MiB；随后 5000 份覆盖不同版本与
// 冲刺的报告总耗时 < 3s。固定随机种子，可重复。
func TestPerformanceScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scale test in -short mode")
	}
	rng := rand.New(rand.NewSource(26100306))

	const (
		numSprints   = 500
		numStories   = 50000
		numFacts     = 200000
		numHotFacts  = 20000 // 集中在一个故事上的事实
		numAmend     = 50000 // 更正 + 撤销
		numReports   = 5000
		maxSubmitDur = 60 * time.Second
		maxQueryDur  = 3 * time.Second
		maxHeapBytes = 512 << 20
	)

	l := NewLedger()
	for i := 0; i < numSprints; i++ {
		id := SprintID(fmt.Sprintf("SP%04d", i))
		if err := l.DefineSprint(id, day0.AddDate(0, 0, SprintDays*i)); err != nil {
			t.Fatalf("DefineSprint: %v", err)
		}
	}
	sprintOf := func(i int) SprintID { return SprintID(fmt.Sprintf("SP%04d", i)) }

	factIDs := make([]FactID, 0, numFacts)
	start := time.Now()

	// 热点故事：2 万条事实集中在同一冲刺窗口内。
	hotSprint := sprintOf(250)
	hotBase := day0.AddDate(0, 0, SprintDays*250)
	for i := 0; i < numHotFacts; i++ {
		f := FactID(fmt.Sprintf("hot-%d", i))
		at := hotBase.Add(time.Duration(i) * time.Minute)
		st := State{Sprint: hotSprint, Points: rng.Intn(9), Done: rng.Intn(4) == 0}
		if _, err := l.AddFact(f, "HOT", at, st); err != nil {
			t.Fatalf("add hot fact: %v", err)
		}
		factIDs = append(factIDs, f)
	}

	// 其余事实均匀散布：每个故事约 3-4 条，归属随机冲刺。
	rest := numFacts - numHotFacts
	for i := 0; i < rest; i++ {
		story := StoryID(fmt.Sprintf("ST%05d", rng.Intn(numStories)))
		sp := rng.Intn(numSprints)
		at := day0.AddDate(0, 0, SprintDays*sp).
			Add(time.Duration(rng.Intn(SprintDays*24*60)) * time.Minute).
			Add(time.Duration(i) * time.Nanosecond) // 保证同故事业务时刻互异
		st := State{Sprint: sprintOf(sp), Points: rng.Intn(9), Done: rng.Intn(4) == 0}
		if rng.Intn(20) == 0 {
			st.Sprint = "" // 少量未入冲刺
		}
		f := FactID(fmt.Sprintf("f%d", i))
		if _, err := l.AddFact(f, story, at, st); err != nil {
			t.Fatalf("add fact %d: %v", i, err)
		}
		factIDs = append(factIDs, f)
	}

	// 5 万次更正 / 撤销（更正 7 : 撤销 3）。
	retracted := make(map[FactID]bool, numAmend)
	for i := 0; i < numAmend; i++ {
		var f FactID
		for {
			f = factIDs[rng.Intn(len(factIDs))]
			if !retracted[f] {
				break
			}
		}
		if rng.Intn(10) < 7 {
			st := State{Sprint: sprintOf(rng.Intn(numSprints)), Points: rng.Intn(9), Done: rng.Intn(3) == 0}
			if _, err := l.CorrectFact(f, st); err != nil {
				t.Fatalf("correct %d: %v", i, err)
			}
		} else {
			if _, err := l.RetractFact(f); err != nil {
				t.Fatalf("retract %d: %v", i, err)
			}
			retracted[f] = true
		}
	}
	submitDur := time.Since(start)
	if submitDur > maxSubmitDur {
		t.Fatalf("submissions took %v, want < %v", submitDur, maxSubmitDur)
	}

	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	if ms.HeapAlloc > maxHeapBytes {
		t.Fatalf("heap alloc = %d MiB, want < 512 MiB", ms.HeapAlloc>>20)
	}

	// 5000 份报告：随机冲刺 × 随机观察版本（含早期版本）。
	start = time.Now()
	var checksum int
	for i := 0; i < numReports; i++ {
		sp := sprintOf(rng.Intn(numSprints))
		v := Version(rng.Uint64() % (uint64(l.Version()) + 1))
		r, err := l.Report(sp, v)
		if err != nil {
			t.Fatalf("report %d: %v", i, err)
		}
		checksum += r.OpeningCommitment + r.Days[13].Remaining
	}
	queryDur := time.Since(start)
	if queryDur > maxQueryDur {
		t.Fatalf("5000 reports took %v, want < %v", queryDur, maxQueryDur)
	}

	t.Logf("submissions: %d facts + %d amends in %v", numFacts, numAmend, submitDur)
	t.Logf("heap alloc: %d MiB", ms.HeapAlloc>>20)
	t.Logf("reports: %d in %v (%.0f/s), checksum=%d",
		numReports, queryDur, float64(numReports)/queryDur.Seconds(), checksum)
}
