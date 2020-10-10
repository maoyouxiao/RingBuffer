package buffer

import (
	"io"
	"sync"
)

type Slicer struct {
	i int
	l int
}

func add(size, a, b int) int {
	if a+b >= size {
		return (a + b) - size
	}
	return a + b
}

func (s *Slicer) Get(data []byte) ([]byte, []byte) {
	if s.l == 0 {
		return nil, nil
	}
	e := add(len(data), s.i, s.l)
	if e == 0 {
		return data[s.i:], nil
	}
	if e > s.i {
		return data[s.i:e], nil
	}
	return data[s.i:], data[:e]
}

type RingBuffer struct {
	data []byte
	size int
	used *Slicer
	free *Slicer
	cond *sync.Cond
	live bool
}

func NewRingBuffer(data []byte) *RingBuffer {
	buf := new(RingBuffer)
	buf.data = data
	buf.size = len(data)
	buf.used = new(Slicer)
	buf.used.i = 0
	buf.used.l = 0
	buf.free = new(Slicer)
	buf.free.i = 0
	buf.free.l = len(data)
	buf.cond = sync.NewCond(new(sync.Mutex))
	buf.live = true

	return buf
}

func (buf *RingBuffer) Len() int {
	return buf.used.l
}

func (buf *RingBuffer) Free() int {
	return buf.free.l
}

func (buf *RingBuffer) Size() int {
	return buf.size
}

func (buf *RingBuffer) Read(data []byte) (n int, err error) {
	buf.cond.L.Lock()
	defer func() {
		buf.cond.Signal()
		buf.cond.L.Unlock()
	}()

	if !buf.live && buf.used.l == 0 {
		return 0, io.EOF
	}
	if buf.used.l == 0 {
		return 0, nil
	}
	start, end := buf.used.Get(buf.data)
	n = copy(data, start)
	if end != nil {
		n += copy(data[n:], end)
	}
	buf.used.i = add(buf.size, buf.used.i, n)
	buf.used.l -= n
	buf.free.l += n

	return n, nil
}

func (buf *RingBuffer) Write(data []byte) (n int, err error) {
	buf.cond.L.Lock()
	defer buf.cond.L.Unlock()

	if !buf.live {
		return 0, io.ErrClosedPipe
	}
	if buf.size < len(data) {
		data = data[:buf.size]
		err = io.ErrShortWrite
	}
	for buf.free.l < len(data) {
		buf.cond.Wait()
		if !buf.live {
			return 0, io.ErrClosedPipe
		}
	}
	start, end := buf.free.Get(buf.data)
	n = copy(start, data)
	if end != nil {
		n += copy(end, data[n:])
	}
	buf.free.i = add(buf.size, buf.free.i, n)
	buf.free.l -= n
	buf.used.l += n

	return n, nil
}

func (buf *RingBuffer) Close() (err error) {
	buf.cond.L.Lock()
	defer func() {
		buf.cond.Broadcast()
		buf.cond.L.Unlock()
	}()
	buf.live = false
	return
}
