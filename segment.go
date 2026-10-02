package burndown

import "time"

// never 表示段在观察版本轴上尚未关闭。
const never = ^uint64(0)

// neverTime 表示业务时间区间向右无限延伸（故事当前最后一条事实之后）。
var neverTime = time.Unix(1<<62, 0).UTC()

// segment 表示某故事在某段业务时间区间内、某段观察版本区间内，
// 以 (points, done) 状态属于某冲刺。
type segment struct {
	btStart time.Time // 业务区间 [btStart, btEnd)
	btEnd   time.Time // neverTime 表示向右无限延伸
	points  int
	done    bool
	vStart  uint64 // 版本区间 [vStart, vEnd)
	vEnd    uint64
}

// visibleAt 报告版本 v 是否能看到该段。
func (s *segment) visibleAt(v uint64) bool {
	return s.vStart <= v && v < s.vEnd
}

// sprintLog 是单个冲刺的追加式段日志。
type sprintLog struct {
	start time.Time
	segs  []*segment
}

// append 追加一个从版本 v 起可见的开放段。
func (sl *sprintLog) append(btStart, btEnd time.Time, st State, v uint64) *segment {
	seg := &segment{btStart: btStart, btEnd: btEnd, points: st.Points, done: st.Done, vStart: v, vEnd: never}
	sl.segs = append(sl.segs, seg)
	return seg
}
