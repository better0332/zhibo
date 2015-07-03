package queue

import (
	"net/http"
	"sync"
	"sync/atomic"
	"unsafe"
)

type StreamQueue struct {
	rlock sync.RWMutex // lock protect resp
	resp  *http.Response

	bfbData *Node

	tail uintptr // // operation by atomic, convert from *Node->unsafe.Pointer->uintptr
}

func NewStreamQueue() *StreamQueue {
	n1, n2 := &Node{}, &Node{}
	n1.Lock()
	n2.Lock()
	q := &StreamQueue{bfbData: n1, tail: uintptr(unsafe.Pointer(n2))}
	q.rlock.Lock()
	return q
}

func (q *StreamQueue) PushResp(resp *http.Response) {
	q.resp = resp
	q.rlock.Unlock()
}

func (q *StreamQueue) PeekResp() *http.Response {
	q.rlock.RLock()
	defer q.rlock.RUnlock()

	return q.resp
}

func (q *StreamQueue) PushBFB(bfb []byte) {
	q.bfbData.data = bfb
	q.bfbData.Unlock()
}

func (q *StreamQueue) PeekBFB() []byte {
	q.bfbData.RLock()
	defer q.bfbData.RUnlock()

	return q.bfbData.data
}

func (q *StreamQueue) Push(data []byte) {
	tail := (*Node)(unsafe.Pointer(q.tail))
	tail.data = data
	defer tail.Unlock()

	if data != nil {
		n := &Node{}
		n.Lock()

		atomic.StoreUintptr(&q.tail, uintptr(unsafe.Pointer(n)))
	}
	return
}

func (q *StreamQueue) Peek() []byte {
	tail := (*Node)(unsafe.Pointer(atomic.LoadUintptr(&q.tail)))

	tail.RLock()
	defer tail.RUnlock()

	return tail.data
}
