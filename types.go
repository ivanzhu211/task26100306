// Package burndown 提供冲刺历史燃尽核算能力。
//
// 上层按到达顺序提交故事事实（新增、更正、撤销），每次提交产生一个
// 递增的观察版本。报告按（冲刺, 观察版本）查询，旧版本报告不受后续
// 提交影响。全部数据保存在进程内存中。
package burndown

import "time"

// Version 是观察版本号，每次成功提交递增，从 1 开始。
// 版本 0 表示尚无任何提交。
type Version uint64

// SprintID 冲刺编号，由上层提供。
type SprintID string

// StoryID 故事编号，由上层提供。
type StoryID string

// FactID 事实稳定标识，由上层提供，全局唯一。
type FactID string

// SprintDays 冲刺长度（天），定义不可修改。
const SprintDays = 14

// State 是故事在某业务时刻的完整状态。
type State struct {
	// Sprint 为所属冲刺；空串表示未入冲刺。
	Sprint SprintID
	// Points 故事点数，必须非负。
	Points int
	// Done 是否完成。
	Done bool
}

// DayResult 是某一天的燃尽结果。
type DayResult struct {
	// Remaining 日终剩余点数：日终仍属于本冲刺且未完成的故事点数之和。
	Remaining int
	// NetScopeChange 相对开场承诺的范围净变化：日终总范围减开场承诺。
	NetScopeChange int
}

// Report 是某冲刺在某观察版本下的燃尽报告。
type Report struct {
	Sprint  SprintID
	Version Version
	// Start 冲刺开始时刻（UTC 零点）。
	Start time.Time
	// OpeningCommitment 开场承诺点数：冲刺开始时刻已经生效、且属于
	// 本冲刺的故事点数之和（包含已完成故事）。
	OpeningCommitment int
	// Days 为 14 天的逐日结果，Days[0] 为第 1 天。
	Days [SprintDays]DayResult
}
