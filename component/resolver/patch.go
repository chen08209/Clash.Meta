package resolver

import "context"

const (
	InitiatorApp    = "app"
	InitiatorRule   = "rule"
	InitiatorDirect = "direct"
	InitiatorProxy  = "proxy"
	InitiatorOther  = "other"
)

type initiatorKey struct{}

func WithInitiator(ctx context.Context, initiator string) context.Context {
	return context.WithValue(ctx, initiatorKey{}, initiator)
}

func InitiatorFrom(ctx context.Context) string {
	initiator, _ := ctx.Value(initiatorKey{}).(string)
	return initiator
}
