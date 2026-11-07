//go:build !windows

package testnet

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

func openRuntimeLock(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("state already in use: %w", err)
	}
	return f, nil
}
func unlockRuntime(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_UN) }
func preserveOwner(f *os.File, st os.FileInfo) error {
	if stat, ok := st.Sys().(*syscall.Stat_t); ok && os.Geteuid() == 0 {
		return f.Chown(int(stat.Uid), int(stat.Gid))
	}
	return nil
}
