package sync2

type GroupResult int

const (
	// Result is not from callback as call was canceled while waiting.
	GroupCanceled GroupResult = iota
	// Result is from callback and is not shared with any other routines.
	GroupExclusive
	// Result is from callback and is shared with other routines in the group.
	GroupShared
)
