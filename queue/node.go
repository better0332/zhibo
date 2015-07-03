package queue

import "sync"

type Node struct {
	sync.RWMutex
	data []byte
	next *Node
}
