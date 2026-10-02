package burndown

import (
	"fmt"
	"time"
)

// day 时长。
const dayDur = 24 * time.Hour

// Report 返回指定冲刺在指定观察版本的燃尽报告。
// 只使用截至该版本已收到的事实及其当时最新修订。
func (l *Ledger) Report(sprintID int64, version uint64) (Report, error) {
	sl, ok := l.sprints[sprintID]
	if !ok {
		return Report{}, fmt.Errorf("burndown: sprint %d not defined", sprintID)
	}
	if version > l.version {
		return Report{}, fmt.Errorf("burndown: version %d not reached yet (current %d)", version, l.version)
	}
	rep := Report{SprintID: sprintID, Version: version}
	t0 := sl.start
	for _, seg := range sl.segs {
		if !seg.visibleAt(version) {
			continue
		}
		// 开场承诺：开始时刻已生效的段，btStart <= t0 < btEnd。
		if !seg.btStart.After(t0) && seg.btEnd.After(t0) {
			rep.Opening += seg.points
		}
		// 第 d 天日终：次日零点之前的状态，恰在零点生效的归次日，
		// 即 btStart < t <= btEnd，t = t0 + d 天。
		for d := 1; d <= SprintDays; d++ {
			t := t0.Add(time.Duration(d) * dayDur)
			if seg.btStart.Before(t) && !seg.btEnd.Before(t) {
				rep.Days[d-1].ScopeNetChange += seg.points
				if !seg.done {
					rep.Days[d-1].Remaining += seg.points
				}
			}
		}
	}
	for d := 0; d < SprintDays; d++ {
		rep.Days[d].ScopeNetChange -= rep.Opening
	}
	return rep, nil
}
