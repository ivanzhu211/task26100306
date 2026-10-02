// Package burndown 提供冲刺历史燃尽核算能力。
//
// 上层以任意到达顺序提交故事事实（新增、更正、撤销），每次提交产生一个
// 递增的观察版本。报告按（冲刺, 观察版本）查询，可复现任一历史版本。
package burndown

import "time"

// SprintDays 是冲刺长度（天）。冲刺从 UTC 零点开始，包含开始时刻、
// 不包含结束时刻。
const SprintDays = 14

// NoSprint 表示故事不属于任何冲刺。
const NoSprint int64 = -1

// State 是故事在某业务时刻的完整状态。
type State struct {
	// SprintID 为所属冲刺编号，NoSprint 表示未入冲刺。
	SprintID int64
	// Points 为故事点数。
	Points int
	// Done 表示是否完成。
	Done bool
}

// Fact 是上层提交的一条故事事实。
type Fact struct {
	// FactID 是事实的稳定标识，全局唯一。
	FactID string
	// StoryID 是故事编号。同一故事的业务时刻互异。
	StoryID string
	// Time 是业务时刻。
	Time time.Time
	// State 是该业务时刻起的完整状态。
	State State
}

// DayResult 是某一天的燃尽结果。
type DayResult struct {
	// Remaining 为日终剩余点数（仍属本冲刺且未完成）。
	Remaining int
	// ScopeNetChange 为日终总范围相对开场承诺的净变化。
	ScopeNetChange int
}

// Report 是某冲刺在某观察版本的燃尽报告。
type Report struct {
	SprintID int64
	Version  uint64
	// Opening 为开场承诺：冲刺开始时刻已生效且属于本冲刺的故事点数之和，
	// 包含已完成故事。
	Opening int
	// Days 为第 1..14 天的逐日结果。
	Days [SprintDays]DayResult
}
