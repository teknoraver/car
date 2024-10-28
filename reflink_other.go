//go:build !linux

package main

import "os"

func reflinkToArchive(*os.File, *os.File, uint64) error {
	return reflinkError
}

func reflinkFromArchive(*os.File, *os.File, uint64) error {
	return reflinkError
}
