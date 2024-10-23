//go:build linux

package main

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func reflinkToArchive(inFd *os.File, archive *os.File, size uint64) error {
	offset, err := archive.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	if size >= cowAlignment {
		fcrange := unix.FileCloneRange{
			Src_fd:      int64(inFd.Fd()),
			Src_length:  size & ^uint64(cowMask),
			Dest_offset: uint64(offset),
		}

		err = unix.IoctlFileCloneRange(int(archive.Fd()), &fcrange)
		if err != nil {
			return err
		}

		_, err = inFd.Seek(int64(fcrange.Src_length), io.SeekStart)
		if err != nil {
			return err
		}

		_, err = archive.Seek(0, io.SeekEnd)
	}

	return err

}

func reflinkFromArchive(archive *os.File, outFd *os.File, size uint64) error {
	offset, err := archive.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	fcrange := unix.FileCloneRange{
		Src_fd:     int64(archive.Fd()),
		Src_offset: uint64(offset),
		Src_length: round4k(size),
	}

	err = unix.IoctlFileCloneRange(int(outFd.Fd()), &fcrange)
	if err != nil {
		return err
	}

	err = outFd.Truncate(int64(size))
	if err != nil {
		return err
	}

	_, err = archive.Seek(int64(size), io.SeekCurrent)

	return err
}
