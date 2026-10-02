package burndown

import "time"

// factRecord 是一条事实在账本内的记录。
type factRecord struct {
	id    string
	bt    time.Time // 业务时刻
	state State     // 当前最新状态（随更正变化）
	// revoked 表示该事实已被撤销。
	revoked bool
	// seg 是该事实当前生效状态对应的开放段；状态不属于任何冲刺时为 nil。
	seg *segment
}

// story 保存单个故事的内部状态：按业务时刻排序的有效事实。
type story struct {
	// order 保存当前有效（未撤销）的事实，按 bt 升序。
	order []*factRecord
}

// locate 返回 bt 在 order 中的位置；found 表示恰好命中。
func (s *story) locate(bt time.Time) (idx int, found bool) {
	i, j := 0, len(s.order)
	for i < j {
		h := (i + j) / 2
		if s.order[h].bt.Before(bt) {
			i = h + 1
		} else {
			j = h
		}
	}
	if i < len(s.order) && s.order[i].bt.Equal(bt) {
		return i, true
	}
	return i, false
}

// endAfter 返回 order 中 idx 之后一条事实的业务时刻，没有则为 neverTime。
func (s *story) endAfter(idx int) time.Time {
	if idx+1 < len(s.order) {
		return s.order[idx+1].bt
	}
	return neverTime
}

// closeSeg 在版本 v 关闭 rec 当前的开放段。
func closeSeg(rec *factRecord, v uint64) {
	if rec.seg != nil {
		rec.seg.vEnd = v
		rec.seg = nil
	}
}

// openSeg 为 rec 的当前状态开启段 [rec.bt, end)，追加到对应冲刺日志。
func (l *Ledger) openSeg(rec *factRecord, end time.Time, v uint64) {
	if rec.state.SprintID == NoSprint {
		rec.seg = nil
		return
	}
	rec.seg = l.sprints[rec.state.SprintID].append(rec.bt, end, rec.state, v)
}

// add 把 rec 插入时间线，并重开受影响的前驱段。
func (l *Ledger) addFact(s *story, rec *factRecord, v uint64) {
	idx, _ := s.locate(rec.bt)
	s.order = append(s.order, nil)
	copy(s.order[idx+1:], s.order[idx:])
	s.order[idx] = rec
	if idx > 0 {
		prev := s.order[idx-1]
		closeSeg(prev, v)
		l.openSeg(prev, rec.bt, v)
	}
	l.openSeg(rec, s.endAfter(idx), v)
}

// correct 用新状态替换 rec 的当前状态，业务时刻不变。
func (l *Ledger) correctFact(s *story, rec *factRecord, st State, v uint64) {
	idx, _ := s.locate(rec.bt)
	closeSeg(rec, v)
	rec.state = st
	l.openSeg(rec, s.endAfter(idx), v)
}

// revoke 撤销 rec：关闭其段并移出时间线，前驱状态延续到其后继。
func (l *Ledger) revokeFact(s *story, rec *factRecord, v uint64) {
	idx, _ := s.locate(rec.bt)
	end := s.endAfter(idx)
	closeSeg(rec, v)
	copy(s.order[idx:], s.order[idx+1:])
	s.order[len(s.order)-1] = nil
	s.order = s.order[:len(s.order)-1]
	if idx > 0 {
		prev := s.order[idx-1]
		closeSeg(prev, v)
		l.openSeg(prev, end, v)
	}
}
