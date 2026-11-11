package store

import "sync"

type Store struct {
	mu sync.RWMutex
}
