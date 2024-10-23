//go:build !linux

package main

import (
	"errors"
	"os"
)

func reflinkToArchive(*os.File, *os.File, uint64) error {
	return errors.New("reflink not supported on this platform")
}

func reflinkFromArchive(*os.File, *os.File, uint64) error {
	return errors.New("reflink not supported on this platform")
}
