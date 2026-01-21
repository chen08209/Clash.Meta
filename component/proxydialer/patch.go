package proxydialer

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// A dialer-proxy can lead back to its own proxy through a group, and the dial
// then recurses until the stack overflows and takes the process with it. A
// dialer asked again for the same address within one dial is such a loop, so
// that dial fails instead.
var ErrDialerLoop = errors.New("dialer-proxy loop")

type LoopNotify func(proxyName string)

var DefaultLoopNotify LoopNotify

type dialStep struct {
	proxyName string
	address   string
}

type dialStepsKey struct{}

func enterDial(ctx context.Context, proxyName, address string) (context.Context, error) {
	steps, _ := ctx.Value(dialStepsKey{}).([]dialStep)
	step := dialStep{proxyName: proxyName, address: address}
	if slices.Contains(steps, step) {
		if notify := DefaultLoopNotify; notify != nil {
			notify(proxyName)
		}
		return ctx, fmt.Errorf("%w: dialing %s through %s comes back to it", ErrDialerLoop, address, proxyName)
	}
	return context.WithValue(ctx, dialStepsKey{}, append(slices.Clip(steps), step)), nil
}
