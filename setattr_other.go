//go:build !linux

package main

func setTimes(string, int64) error {
	return nil
}
