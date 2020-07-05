package sync2

import "context"

type CallCtxGroup struct {
	callGroup CallGroup
}

type callResult struct {
	val interface{}
	err error
}

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
