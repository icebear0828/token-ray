package applock_test

import (
	"errors"
	"path/filepath"
	"testing"

	"token-ray/internal/applock"
)

func TestTryAcquireRejectsSecondHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard.lock")

	first, err := applock.TryAcquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()

	second, err := applock.TryAcquire(path)
	if !errors.Is(err, applock.ErrAlreadyLocked) {
		if second != nil {
			second.Release()
		}
		t.Fatalf("expected ErrAlreadyLocked, got lock=%v err=%v", second, err)
	}
	if second != nil {
		t.Fatalf("expected no second lock, got %v", second)
	}
}

func TestTryAcquireAllowsAfterRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard.lock")

	first, err := applock.TryAcquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}

	second, err := applock.TryAcquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Release()
}
