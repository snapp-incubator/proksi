package storage

type Storage interface {
	Store(Log) error
}
