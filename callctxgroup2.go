package sync2

import "context"

type CallCtxGroup2 struct {
	callGroup CallGroup
}

type callResult struct {
	val interface{}
	err error
}

func (g *CallCtxGroup2) Do(ctx context.Context, key string, do func() (interface{}, error)) (interface{}, GroupResult, error) {
	res, grouped := g.callGroup.Do(key, ctx.Done(), func() (interface{}, bool) {
		val, err := do()
		if ctx.Err() != nil {
			return callResult{val: val, err: err}, false
		}
		return callResult{val: val, err: err}, true
	})
	if grouped == GroupCanceled {
		return nil, GroupCanceled, ctx.Err()
	}
	res2 := res.(callResult)
	return res2.val, grouped, res2.err
}
