package burndown

import "errors"

var (
	// ErrSprintExists 表示重复定义冲刺。
	ErrSprintExists = errors.New("burndown: sprint already defined")
	// ErrSprintNotDefined 表示冲刺未预先定义。
	ErrSprintNotDefined = errors.New("burndown: sprint not defined")
	// ErrInvalidSprintStart 表示冲刺开始时刻不是 UTC 零点。
	ErrInvalidSprintStart = errors.New("burndown: sprint start must be UTC midnight")
	// ErrFactExists 表示事实标识重复。
	ErrFactExists = errors.New("burndown: fact already exists")
	// ErrFactNotFound 表示事实不存在。
	ErrFactNotFound = errors.New("burndown: fact not found")
	// ErrFactRetracted 表示事实已被撤销。
	ErrFactRetracted = errors.New("burndown: fact already retracted")
	// ErrDuplicateBusinessTime 表示同一故事的业务时刻重复。
	ErrDuplicateBusinessTime = errors.New("burndown: duplicate business time for story")
	// ErrNegativePoints 表示故事点数为负。
	ErrNegativePoints = errors.New("burndown: negative story points")
	// ErrVersionOutOfRange 表示观察版本超出已产生的版本范围。
	ErrVersionOutOfRange = errors.New("burndown: version out of range")
)
