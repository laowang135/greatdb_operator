package manager

import (
	"errors"
)

var (
	HandlingLimitErr = errors.New("handling speed limit")
	SkipErr          = errors.New("synchronizing, skipping")
)
