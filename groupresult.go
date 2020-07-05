package sync2

type GroupResult int

const (
	GroupCanceled GroupResult = iota
	GroupExclusive
	GroupShared
)
