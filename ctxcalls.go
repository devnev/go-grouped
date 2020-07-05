package grouped

import "context"

// CtxCalls allows batching together calls with the same key to share the result of executing
// only one of the callbacks in the batch.
type CtxCalls struct {
	callGroup Calls
}

// Do starts or joins the call group for the given key, waiting for a member of the group to complete
// its callback and return a result that should be accepted by the group. If the executed callback
// panics or its context is done, a different member's callback will be invoked for the group, and
// so on until an invoked callback completes successfully.
func (g *CtxCalls) Do(ctx context.Context, key string, do func() (interface{}, error)) (interface{}, Status, error) {
	res, grouped := g.callGroup.Do(key, ctx.Done(), func() (interface{}, bool) {
		val, err := do()
		return callResult{val: val, err: err}, ctx.Err() == nil
	})
	if grouped == Canceled {
		return nil, Canceled, ctx.Err()
	}
	res2 := res.(callResult)
	return res2.val, grouped, res2.err
}

type callResult struct {
	val interface{}
	err error
}
