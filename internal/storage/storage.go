package storage

// Storage defines the behavior of log storage
type Storage interface {
	// Store is the action of storing
	Store(Log) error
}
