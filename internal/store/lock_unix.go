//go:build darwin || linux

package store

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryCredentialFileLock(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return false, err
}

func unlockCredentialFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
