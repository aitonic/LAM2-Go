//go:build !darwin && !linux

package storage

import (
	"context"
	"errors"
)

func Lock(context.Context, string) (func(), error) {
	return nil, errors.New("LAM 2 currently supports macOS and Linux file locking only")
}
