package burndown

import (
	"errors"
	"fmt"
	"time"
)

// Ledger 是冲刺燃尽核算账本。全部数据保存在进程内存。
// 调用方需串行调用，本类型不提供并发安全。
type Ledger struct {
	version uint64
	sprints map[int64]*sprintLog
	stories map[string]*story
	facts   map[string]*factRecord
	owners  map[string]*story // factID -> 所属故事
}

// New 创建一个空账本。
func New() *Ledger {
	return &Ledger{
		sprints: make(map[int64]*sprintLog),
		stories: make(map[string]*story),
		facts:   make(map[string]*factRecord),
		owners:  make(map[string]*story),
	}
}

// Version 返回当前观察版本。每次成功提交使其递增。
func (l *Ledger) Version() uint64 {
	return l.version
}

// DefineSprint 定义一个冲刺。start 必须是 UTC 零点。
// 冲刺定义不可修改：重复定义同一编号返回错误。
func (l *Ledger) DefineSprint(id int64, start time.Time) error {
	if id == NoSprint {
		return fmt.Errorf("burndown: sprint id %d is reserved", NoSprint)
	}
	if _, ok := l.sprints[id]; ok {
		return fmt.Errorf("burndown: sprint %d already defined", id)
	}
	if start.Location() != time.UTC ||
		start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 || start.Nanosecond() != 0 {
		return fmt.Errorf("burndown: sprint %d start %v is not a UTC midnight", id, start)
	}
	l.sprints[id] = &sprintLog{start: start}
	return nil
}

// checkSprint 校验状态引用的冲刺已定义。
func (l *Ledger) checkSprint(st State) error {
	if st.SprintID == NoSprint {
		return nil
	}
	if _, ok := l.sprints[st.SprintID]; !ok {
		return fmt.Errorf("burndown: sprint %d not defined", st.SprintID)
	}
	return nil
}

// SubmitAdd 提交一条新事实，返回新的观察版本。
func (l *Ledger) SubmitAdd(f Fact) (uint64, error) {
	if f.FactID == "" || f.StoryID == "" {
		return 0, errors.New("burndown: fact id and story id must be non-empty")
	}
	if _, ok := l.facts[f.FactID]; ok {
		return 0, fmt.Errorf("burndown: fact %q already exists", f.FactID)
	}
	if err := l.checkSprint(f.State); err != nil {
		return 0, err
	}
	s := l.stories[f.StoryID]
	if s == nil {
		s = &story{}
		l.stories[f.StoryID] = s
	}
	if _, found := s.locate(f.Time); found {
		return 0, fmt.Errorf("burndown: story %q already has a fact at %v", f.StoryID, f.Time)
	}
	rec := &factRecord{id: f.FactID, bt: f.Time, state: f.State}
	l.version++
	l.addFact(s, rec, l.version)
	l.facts[f.FactID] = rec
	l.owners[f.FactID] = s
	return l.version, nil
}

// liveFact 取出一条尚未撤销的事实及其所属故事。
func (l *Ledger) liveFact(factID string) (*factRecord, *story, error) {
	rec, ok := l.facts[factID]
	if !ok {
		return nil, nil, fmt.Errorf("burndown: fact %q not found", factID)
	}
	if rec.revoked {
		return nil, nil, fmt.Errorf("burndown: fact %q already revoked", factID)
	}
	return rec, l.owners[factID], nil
}

// SubmitCorrect 更正一条尚未撤销的事实，只替换状态，
// 不改变故事编号和业务时刻，返回新的观察版本。
func (l *Ledger) SubmitCorrect(factID string, s State) (uint64, error) {
	if err := l.checkSprint(s); err != nil {
		return 0, err
	}
	rec, st, err := l.liveFact(factID)
	if err != nil {
		return 0, err
	}
	l.version++
	l.correctFact(st, rec, s, l.version)
	return l.version, nil
}

// SubmitRevoke 撤销一条尚未撤销的事实，返回新的观察版本。
// 撤销后前一状态延续到下一条有效事实。
func (l *Ledger) SubmitRevoke(factID string) (uint64, error) {
	rec, st, err := l.liveFact(factID)
	if err != nil {
		return 0, err
	}
	l.version++
	l.revokeFact(st, rec, l.version)
	rec.revoked = true
	return l.version, nil
}
