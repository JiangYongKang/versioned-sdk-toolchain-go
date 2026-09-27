package generator

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
)

// rootLocks serializes generation attempts to the same output root inside
// one process. The advisory file lock serializes across processes.
var rootLocks sync.Map // string -> *sync.Mutex

func rootMutex(root string) *sync.Mutex {
	m, _ := rootLocks.LoadOrStore(root, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// fileLock is an advisory, whole-directory lock implemented with flock(2).
type fileLock struct {
	f *os.File
}

// acquireFileLock creates the lock file (and its directory) and takes an
// exclusive, blocking lock. The returned lock must be released.
func acquireFileLock(root string) (*fileLock, error) {
	dir := filepath.Join(root, sdkDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, sdkerrors.New(sdkerrors.KindGeneration,
			"cannot create output state directory").Wrap(err)
	}
	path := filepath.Join(dir, "lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, sdkerrors.New(sdkerrors.KindGeneration,
			"cannot open generation lock file").Wrap(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, sdkerrors.New(sdkerrors.KindGeneration,
			"cannot acquire exclusive generation lock").Wrap(err)
	}
	return &fileLock{f: f}, nil
}

func (l *fileLock) release() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	return l.f.Close()
}
