package burndown

import "sort"

// computeReport 计算冲刺在观察版本 v 下的报告。
//
// 对每个候选故事，按业务时刻升序单次扫描其事实，依次推进开场时刻
// 与 14 个日终检查点，再按规则聚合：
//   - 开场承诺：开始时刻（含恰在开始时刻生效的事实）属于本冲刺的
//     故事点数之和，包含已完成故事；
//   - 日终剩余：日终仍属于本冲刺且未完成的故事点数之和；
//   - 日终总范围：日终仍属于本冲刺的全部故事点数之和；
//   - 范围净变化 = 日终总范围 - 开场承诺。
//
// 日终取次日 UTC 零点之前的状态：恰在次日零点生效的事实归次日。
// 业务时刻不早于冲刺结束的事实不影响任何检查点，扫描自然终止。
func computeReport(id SprintID, def *sprintDef, l *Ledger, v Version) Report {
	rep := Report{Sprint: id, Version: v, Start: def.start}
	var scope [SprintDays]int

	for _, story := range def.candList {
		facts := l.stories[story].facts
		n := len(facts)
		// 二分定位首个晚于开场时刻的事实；此前最后一个有效
		// 事实即为开场状态，避免扫描窗口前的大量历史事实。
		i := sort.Search(n, func(i int) bool { return facts[i].at.After(def.start) })
		var cur State
		active := false
		for j := i - 1; j >= 0; j-- {
			if st, ok := facts[j].stateAt(v); ok {
				cur, active = st, true
				break
			}
		}
		if active && cur.Sprint == id {
			rep.OpeningCommitment += cur.Points
		}

		// 第 day 天日终：业务时刻 < start+day 天 的最后有效事实。
		for day := 1; day <= SprintDays; day++ {
			dayEnd := def.start.AddDate(0, 0, day)
			for i < n && facts[i].at.Before(dayEnd) {
				if st, ok := facts[i].stateAt(v); ok {
					cur, active = st, true
				}
				i++
			}
			if !active || cur.Sprint != id {
				continue
			}
			scope[day-1] += cur.Points
			if !cur.Done {
				rep.Days[day-1].Remaining += cur.Points
			}
		}
	}

	for day := 0; day < SprintDays; day++ {
		rep.Days[day].NetScopeChange = scope[day] - rep.OpeningCommitment
	}
	return rep
}
