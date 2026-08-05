package fileextract

import "testing"

func TestDocLockerForceWriteBusy(t *testing.T) {
	l := NewDocLocker()
	hash := "abc"

	if !l.TryLock(hash) {
		t.Fatal("first write lock should succeed")
	}
	if l.TryLock(hash) {
		t.Fatal("second write lock should fail")
	}
	if l.TryRLock(hash) {
		t.Fatal("read lock should fail while write held")
	}
	l.Unlock(hash)

	if !l.TryRLock(hash) {
		t.Fatal("read lock should succeed after unlock")
	}
	if !l.TryRLock(hash) {
		t.Fatal("second read lock should succeed")
	}
	if l.TryLock(hash) {
		t.Fatal("write lock should fail while readers held")
	}
	l.RUnlock(hash)
	l.RUnlock(hash)

	if !l.TryLock(hash) {
		t.Fatal("write lock should succeed after readers release")
	}
	l.Unlock(hash)
}
