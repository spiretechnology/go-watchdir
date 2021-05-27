package watchdir

// Op defines an operation that took place on the watch directory
type Op uint8

const (
	Add    Op = 1 << 0
	Remove    = 1 << 1
)
