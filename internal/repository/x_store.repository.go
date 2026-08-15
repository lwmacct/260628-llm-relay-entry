package repository

import "github.com/uptrace/bun"

type Store struct {
	db *bun.DB
}

func NewStore(db *bun.DB) *Store {
	if db == nil {
		panic("NewStore: db is nil")
	}
	return &Store{db: db}
}
