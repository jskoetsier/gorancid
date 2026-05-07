package collect

// CollectStatus is the outcome of a collection attempt.
type CollectStatus int

const (
	StatusSuccess CollectStatus = iota
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

// CollectionError is a structured error for one device failure, usable with errors.Is/As.
type CollectionError struct {
	Device string
	Err    error
}

func (e CollectionError) Error() string {
	if e.Err != nil {
		return e.Device + ": " + e.Err.Error()
	}
	return e.Device + ": unknown error"
}

func (e CollectionError) Unwrap() error { return e.Err }
