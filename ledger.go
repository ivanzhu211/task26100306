package burndown

import "time"

// sprintDef 是冲刺定义及其候选故事集合。
type sprintDef struct {
	start time.Time
	// candidates 是任何修订版本中曾声明属于本冲刺的故事集合
	// （去重判定用），candList 为同一集合的切片形式（迭代用）。
	// 只增不减；报告时重放过滤，保证历史版本正确。
	candidates map[StoryID]struct{}
	candList   []StoryID
}

// end 返回冲刺结束时刻（不含）。
func (d *sprintDef) end() time.Time { return d.start.AddDate(0, 0, SprintDays) }

// Ledger 是燃尽核算账本，保存全部故事事实与修订历史。
// 不是并发安全的；调用方需串行访问。
type Ledger struct {
	sprints map[SprintID]*sprintDef
	stories map[StoryID]*storyFacts
	facts   map[FactID]*factRecord
	version Version
}

// NewLedger 创建空账本。
func NewLedger() *Ledger {
	return &Ledger{
		sprints: make(map[SprintID]*sprintDef),
		stories: make(map[StoryID]*storyFacts),
		facts:   make(map[FactID]*factRecord),
	}
}

// Version 返回当前观察版本。
func (l *Ledger) Version() Version { return l.version }

// DefineSprint 预先定义冲刺：编号与开始时刻（UTC 零点）。
// 冲刺从 start 起持续 14 天，含开始时刻、不含结束时刻。
// 必须在任何引用该冲刺的事实提交之前调用。
func (l *Ledger) DefineSprint(id SprintID, start time.Time) error {
	if _, ok := l.sprints[id]; ok {
		return ErrSprintExists
	}
	if utc := start.UTC(); !utc.Equal(start) || utc.Hour() != 0 || utc.Minute() != 0 ||
		utc.Second() != 0 || utc.Nanosecond() != 0 {
		return ErrInvalidSprintStart
	}
	l.sprints[id] = &sprintDef{start: start, candidates: make(map[StoryID]struct{})}
	return nil
}

// validateState 校验状态引用的冲刺已定义、点数非负。
func (l *Ledger) validateState(st State) error {
	if st.Points < 0 {
		return ErrNegativePoints
	}
	if st.Sprint != "" {
		if _, ok := l.sprints[st.Sprint]; !ok {
			return ErrSprintNotDefined
		}
	}
	return nil
}

// indexCandidate 若状态声明属于某冲刺且业务时刻早于冲刺结束，
// 把故事加入该冲刺候选集合。
func (l *Ledger) indexCandidate(story StoryID, at time.Time, st State) {
	if st.Sprint == "" {
		return
	}
	def := l.sprints[st.Sprint]
	if at.Before(def.end()) {
		if _, ok := def.candidates[story]; ok {
			return
		}
		def.candidates[story] = struct{}{}
		def.candList = append(def.candList, story)
	}
}

// AddFact 提交一条新事实：故事 story 在业务时刻 at 的完整状态 st。
// 同一故事的业务时刻（在未撤销事实间）必须互异。返回新观察版本。
func (l *Ledger) AddFact(fact FactID, story StoryID, at time.Time, st State) (Version, error) {
	if _, ok := l.facts[fact]; ok {
		return l.version, ErrFactExists
	}
	if err := l.validateState(st); err != nil {
		return l.version, err
	}
	sf := l.stories[story]
	if sf == nil {
		sf = &storyFacts{}
		l.stories[story] = sf
	}
	if sf.hasActiveAt(at) {
		return l.version, ErrDuplicateBusinessTime
	}
	l.version++
	rec := &factRecord{
		id:    fact,
		story: story,
		at:    at,
		rev:   []factRevision{{version: l.version, state: st}},
	}
	l.facts[fact] = rec
	sf.add(rec)
	l.indexCandidate(story, at, st)
	return l.version, nil
}

// CorrectFact 更正一条尚未撤销的事实，只替换状态，
// 不改变故事编号和业务时刻。返回新观察版本。
func (l *Ledger) CorrectFact(fact FactID, st State) (Version, error) {
	rec, ok := l.facts[fact]
	if !ok {
		return l.version, ErrFactNotFound
	}
	if rec.retracted != 0 {
		return l.version, ErrFactRetracted
	}
	if err := l.validateState(st); err != nil {
		return l.version, err
	}
	l.version++
	rec.rev = append(rec.rev, factRevision{version: l.version, state: st})
	l.indexCandidate(rec.story, rec.at, st)
	return l.version, nil
}

// RetractFact 撤销一条尚未撤销的事实，相当于该事实不再存在，
// 前一状态因此延续到下一条有效事实。返回新观察版本。
func (l *Ledger) RetractFact(fact FactID) (Version, error) {
	rec, ok := l.facts[fact]
	if !ok {
		return l.version, ErrFactNotFound
	}
	if rec.retracted != 0 {
		return l.version, ErrFactRetracted
	}
	l.version++
	rec.retracted = l.version
	return l.version, nil
}

// Report 计算冲刺 sprint 在观察版本 v 下的燃尽报告。
// 只使用截至 v 已收到的事实及其当时最新修订；
// 后续提交不改变旧版本报告。
func (l *Ledger) Report(sprint SprintID, v Version) (Report, error) {
	def, ok := l.sprints[sprint]
	if !ok {
		return Report{}, ErrSprintNotDefined
	}
	if v > l.version {
		return Report{}, ErrVersionOutOfRange
	}
	return computeReport(sprint, def, l, v), nil
}
