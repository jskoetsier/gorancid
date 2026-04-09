package collect

// CollectStatus is the outcome of a collection attempt.
type CollectStatus int

const (
	StatusSuccess   CollectStatus = iota
	StatusFailed
	StatusUnchanged
)

// Result holds the outcome of collecting a single device.
type Result struct {
	Hostname string
	Status   CollectStatus
	Diff     []byte
	Error    error
}