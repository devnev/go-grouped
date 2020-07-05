package sync2

import "context"

// CallCtxGroup allows batching together calls with the same key to share the result of executing
// only one of the callbacks in the batch.
type CallCtxGroup struct {
	callGroup CallGroup
}

// Do starts or joins the call group for the given key, waiting for a member of the group to complete
// its callback and return a result that should be accepted by the group. If the executed callback
// panics or its context is done, a different member's callback will be invoked for the group, and
// so on until an invoked callback completes successfully.
func (g *CallCtxGroup) Do(ctx context.Context, key string, do func() (interface{}, error)) (interface{}, GroupResult, error) {
	res, grouped := g.callGroup.Do(key, ctx.Done(), func() (interface{}, bool) {
		val, err := do()
		return callResult{val: val, err: err}, ctx.Err() == nil
	})
	if grouped == GroupCanceled {
		return nil, GroupCanceled, ctx.Err()
	}
	res2 := res.(callResult)
	return res2.val, grouped, res2.err
}

type callResult struct {
	val interface{}
	err error
}
