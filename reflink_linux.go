//go:build linux

package main

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func reflinkToArchive(inFd *os.File, archive *os.File, size uint64) error {
	// The archive can be non seekable (e.g. a pipe), in this case fall back to classic copy
	offset, err := archive.Seek(0, io.SeekCurrent)
	if err != nil {
		return reflinkError
	}

	// If the file size is less than the minimum allowed by reflink, give up
	if size >= cowAlignment {
		fcrange := unix.FileCloneRange{
			Src_fd:      int64(inFd.Fd()),
			Src_length:  size & ^uint64(cowMask),
			Dest_offset: uint64(offset),
		}

		/* reflink could fail for a lot of reasons (unsupported, different mountpoint etc.)
		 * fallback to a copy in case of non fatal error */
		err = unix.IoctlFileCloneRange(int(archive.Fd()), &fcrange)
		if err != nil {
			return reflinkError
		}

		// Past this point, errors are fatal

		/* reflink does not move the file pointer, seek the input file
		 * to the last reflinked block so we can copy the remainder */
		_, err = inFd.Seek(int64(fcrange.Src_length), io.SeekStart)
		if err != nil {
			return err
		}

		// reflink does not move the file pointer, seek to the end
		_, err = archive.Seek(0, io.SeekEnd)
	}

	return err

}

func reflinkFromArchive(archive *os.File, outFd *os.File, size uint64) error {
	// The archive can be non seekable (e.g. a pipe), in this case fall back to classic copy
	offset, err := archive.Seek(0, io.SeekCurrent)
	if err != nil {
		return reflinkError
	}

	fcrange := unix.FileCloneRange{
		Src_fd:     int64(archive.Fd()),
		Src_offset: uint64(offset),
		Src_length: round4k(size),
	}

	/* reflink could fail for a lot of reasons (unsupported, different mountpoint etc.)
	 * fallback to a copy in case of non fatal error */
	err = unix.IoctlFileCloneRange(int(outFd.Fd()), &fcrange)
	if err != nil {
		return reflinkError
	}

	// Past this point, errors are fatal

	// We rounded up to the block size, truncate the file to remove the excess, if any
	if size&cowMask != 0 {
		err = outFd.Truncate(int64(size))
		if err != nil {
			return err
		}
	}

	// reflink does not move the file pointer, skip the file content
	_, err = archive.Seek(int64(size), io.SeekCurrent)

	return err
}
