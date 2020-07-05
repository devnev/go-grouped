package grouped

type Status int

const (
	// Result is not from callback as call was canceled while waiting.
	Canceled Status = iota
	// Result is from callback and is not shared with any other routines.
	Exclusive
	// Result is from callback and is shared with other routines in the group.
	Shared
)
