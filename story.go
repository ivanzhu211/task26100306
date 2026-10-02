package burndown

import (
	"sort"
	"time"
)

// factRevision 是事实的一次修订：自 version 起（含）生效的状态。
type factRevision struct {
	version Version
	state   State
}

// factRecord 是一条事实及其全部修订历史。
type factRecord struct {
	id        FactID
	story     StoryID
	at        time.Time      // 业务时刻，创建后不变
	rev       []factRevision // 按版本递增，首条为新增
	retracted Version        // 撤销版本；0 表示未撤销
}

// stateAt 返回事实在观察版本 v 下的状态与是否有效存在。
func (f *factRecord) stateAt(v Version) (State, bool) {
	if f.retracted != 0 && f.retracted <= v {
		return State{}, false
	}
	if len(f.rev) == 1 { // 快路径：无更正
		if f.rev[0].version <= v {
			return f.rev[0].state, true
		}
		return State{}, false
	}
	// rev 按版本递增，找最后一个 version <= v 的修订。
	i := sort.Search(len(f.rev), func(i int) bool { return f.rev[i].version > v }) - 1
	if i < 0 {
		return State{}, false
	}
	return f.rev[i].state, true
}

// storyFacts 保存一个故事的全部事实，按业务时刻升序。
type storyFacts struct {
	facts []*factRecord
}

// add 按业务时刻升序插入事实；业务时刻互异由调用方保证。
func (s *storyFacts) add(f *factRecord) {
	i := sort.Search(len(s.facts), func(i int) bool { return !s.facts[i].at.Before(f.at) })
	s.facts = append(s.facts, nil)
	copy(s.facts[i+1:], s.facts[i:])
	s.facts[i] = f
}

// hasActiveAt 报告故事在业务时刻 at 是否已有未撤销事实。
func (s *storyFacts) hasActiveAt(at time.Time) bool {
	for _, f := range s.facts {
		if f.at.Equal(at) && f.retracted == 0 {
			return true
		}
	}
	return false
}
