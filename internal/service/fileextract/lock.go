package fileextract

import (
	"errors"
	"sync"
)

// ErrBusy 同一文档正在识别/强刷，未抢到锁。
var ErrBusy = errors.New("文档正在识别中")

// DocLocker 按 content_hash 的进程内读写锁（无 DB）。
// 读缓存用 RLock；强刷/抽取用写锁（TryLock，抢不到立即失败）。
type DocLocker struct {
	mu    sync.Mutex
	entry map[string]*hashRW
}

type hashRW struct {
	rw sync.RWMutex
}

func NewDocLocker() *DocLocker {
	return &DocLocker{entry: map[string]*hashRW{}}
}

func (l *DocLocker) get(hash string) *hashRW {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entry[hash]
	if !ok {
		e = &hashRW{}
		l.entry[hash] = e
	}
	return e
}

// TryRLock 尝试读锁；失败表示有写者占锁。
func (l *DocLocker) TryRLock(hash string) bool {
	if l == nil {
		return true
	}
	return l.get(hash).rw.TryRLock()
}

func (l *DocLocker) RUnlock(hash string) {
	if l == nil {
		return
	}
	l.get(hash).rw.RUnlock()
}

// TryLock 尝试写锁；失败立即返回 false（不阻塞）。
func (l *DocLocker) TryLock(hash string) bool {
	if l == nil {
		return true
	}
	return l.get(hash).rw.TryLock()
}

func (l *DocLocker) Unlock(hash string) {
	if l == nil {
		return
	}
	l.get(hash).rw.Unlock()
}
