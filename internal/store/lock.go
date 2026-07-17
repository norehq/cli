package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const credentialLockRetryDelay = 25 * time.Millisecond

type CredentialLock struct {
	file *os.File
}

func (s Store) Lock(ctx context.Context) (*CredentialLock, error) {
	path, err := s.Path()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), directoryMode); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, fileMode)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(fileMode); err != nil {
		_ = file.Close()
		return nil, err
	}
	for {
		locked, lockErr := tryCredentialFileLock(file)
		if lockErr != nil {
			_ = file.Close()
			return nil, lockErr
		}
		if locked {
			return &CredentialLock{file: file}, nil
		}
		timer := time.NewTimer(credentialLockRetryDelay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			_ = file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *CredentialLock) Unlock() error {
	if l == nil || l.file == nil {
		return nil
	}
	file := l.file
	l.file = nil
	return errors.Join(unlockCredentialFile(file), file.Close())
}
