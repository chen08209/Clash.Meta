package tunnel

type TunnelStatus int32

const (
	Suspend TunnelStatus = iota
	Inner
	Running
)

func (s TunnelStatus) String() string {
	switch s {
	case Suspend:
		return "suspend"
	case Inner:
		return "inner"
	case Running:
		return "running"
	default:
		return "Unknown"
	}
}
