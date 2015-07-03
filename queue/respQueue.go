package queue

import (
	"net/http"
	"sync"
	"sync/atomic"
)

type RespQueue struct {
	rlock sync.RWMutex // lock protect resp
	resp  *http.Response

	head *Node
	tail *Node

	// operation by atomic
	size int32
	done int32
	del  int32
}

func NewRespQueue() *RespQueue {
	n := &Node{}
	n.Lock()
	q := &RespQueue{head: n, tail: n}
	q.rlock.Lock()
	return q
}

func (q *RespQueue) Del() {
	atomic.StoreInt32(&q.del, 1)
}

func (q *RespQueue) IsDone() bool {
	if atomic.LoadInt32(&q.done) != 0 {
		return true
	} else {
		return false
	}
}

func (q *RespQueue) GetSize() int {
	return int(atomic.LoadInt32(&q.size))
}

func (q *RespQueue) Push(data []byte) bool {
	if atomic.LoadInt32(&q.del) == 1 {
		return false
	}

	tail := q.tail
	tail.data = data

	if data != nil {
		n := &Node{}
		n.Lock()

		tail.next = n
		q.tail = n

		atomic.AddInt32(&q.size, int32(len(data)))
	} else {
		atomic.StoreInt32(&q.done, 1)
	}
	tail.Unlock()

	return true
}

func (q *RespQueue) PushResp(resp *http.Response) {
	q.resp = resp
	q.rlock.Unlock()
}

func (q *RespQueue) Peek(n *Node) ([]byte, *Node) {
	if atomic.LoadInt32(&q.del) == 1 {
		return nil, nil
	}

	if n == nil {
		n = q.head
	}
	n.RLock()
	defer n.RUnlock()

	return n.data, n.next
}

func (q *RespQueue) PeekResp() *http.Response {
	q.rlock.RLock()
	defer q.rlock.RUnlock()

	return q.resp
}
