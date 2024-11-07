package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

var zeroes = make([]byte, cowAlignment)

func (c *car) writeTag(tagType, length uint16, out *os.File, data any) error {
	t := tag{
		Tag:    tagType,
		Length: length,
	}

	err := binary.Write(out, binary.BigEndian, &t)
	if err != nil {
		return err
	}

	if data != nil {
		return binary.Write(out, binary.BigEndian, data)
	}

	return nil
}

func (c *car) writeData(out *os.File, e entry) error {
	if e.size == 0 || e.Mode&unix.S_IFMT != unix.S_IFREG {
		return c.writeTag(tagData, 0, out, nil)
	}

	pd := paddedData{
		Size: e.size,
	}

	// To add padding, we need to know the current offset, so the file must be seekable
	if offset, err := out.Seek(0, io.SeekCurrent); err == nil {
		// tag + paddedData
		const overhead = 4 + 12
		newDataOffset := round4k(uint64(offset + overhead))
		padding := newDataOffset - uint64(offset+overhead)

		pd.Padding = uint32(padding)

		err = c.writeTag(tagData, 12, out, &pd)
		if err != nil {
			return err
		}

		_, err = out.Seek(int64(padding), io.SeekCurrent)
		if err != nil {
			return err
		}
	} else {
		err = c.writeTag(tagData, 12, out, &pd)
		if err != nil {
			return err
		}
	}

	in, err := os.Open(e.localName)
	if err != nil {
		return err
	}
	defer in.Close()

	err = reflinkToArchive(in, out, e.size)
	if err != nil && !errors.Is(err, reflinkError) {
		return err
	}

	_, err = io.Copy(out, in)

	return err
}

func (c *car) writeHeader(out *os.File, e entry) error {
	_, err := out.Write([]byte(cowMagic))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Write error:", err)
		return err
	}

	err = c.writeTag(tagHeader, uint16(binary.Size(e.fixedData)), out, &e.fixedData)
	if err != nil {
		return err
	}

	err = c.writeTag(tagName, uint16(len(e.name)), out, []byte(e.name))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Write error:", err)
		return err
	}

	if e.Mode&unix.S_IFMT == unix.S_IFLNK {
		err = c.writeTag(tagLinkTarget, uint16(len(e.link)), out, []byte(e.link))
		if err != nil {
			return err
		}
	}

	if e.Mode&unix.S_IFMT == unix.S_IFCHR || e.Mode&unix.S_IFMT == unix.S_IFBLK {
		err = c.writeTag(tagDevice, 4, out, e.dev)
		if err != nil {
			return err
		}
	}

	return c.writeData(out, e)
}

func unixMode(mode fs.FileMode) uint32 {
	var unixMode uint32

	switch {
	case mode.IsRegular():
		unixMode = unix.S_IFREG
	case mode.IsDir():
		unixMode = unix.S_IFDIR
	case mode&fs.ModeSymlink != 0:
		unixMode = unix.S_IFLNK
	case mode&fs.ModeNamedPipe != 0:
		unixMode = unix.S_IFIFO
	/* fs.ModeCharDevice bit is contained also in fs.ModeDevice, so order is important */
	case mode&fs.ModeCharDevice != 0:
		unixMode = unix.S_IFCHR
	case mode&fs.ModeDevice != 0:
		unixMode = unix.S_IFBLK
	case mode&fs.ModeSocket != 0:
		unixMode = unix.S_IFSOCK
	}

	if mode&fs.ModeSetuid != 0 {
		unixMode |= unix.S_ISUID
	}
	if mode&fs.ModeSetgid != 0 {
		unixMode |= unix.S_ISGID
	}
	if mode&fs.ModeSticky != 0 {
		unixMode |= unix.S_ISVTX
	}

	return unixMode | uint32(mode.Perm())
}

func (c *car) walker(strip int, p string, statinfo fs.FileInfo, err error, outFd *os.File) error {
	if c.verbose {
		fmt.Fprintln(c.infoFd, p)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error walking", p, err)
		return err
	}

	info := statinfo.Mode()

	switch {
	case info&fs.ModeSocket != 0:
		fallthrough
	case info&fs.ModeIrregular != 0:
		fmt.Fprintln(os.Stderr, "Skipping", p)
		return nil
	}

	storedName := p[strip:]
	if storedName[0] == '/' {
		storedName = storedName[1:]
	}

	entry := entry{
		fixedData: fixedData{
			Mode:  unixMode(info),
			Mtime: statinfo.ModTime().UnixNano(),
		},
		name:      storedName,
		localName: p,
	}

	if sys, ok := statinfo.Sys().(*syscall.Stat_t); ok {
		entry.Uid = sys.Uid
		entry.Gid = sys.Gid
		entry.dev = uint32(sys.Rdev)
	}

	switch {
	case info&fs.ModeNamedPipe != 0:
		break
	case info&fs.ModeDevice != 0:
		fallthrough
	case info&fs.ModeCharDevice != 0:
		break
	case info.IsRegular():
		entry.size = uint64(statinfo.Size())
	case info.IsDir():
	case info&fs.ModeSymlink != 0:
		entry.link, err = os.Readlink(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error reading symlink", p, err)
			return nil
		}
	}

	err = c.writeHeader(outFd, entry)
	if err != nil {
		return err
	}

	return nil
}

func (c *car) walkPaths(paths []string, outFd *os.File) error {
	for _, dir := range paths {
		dir = filepath.Clean(dir)
		err := filepath.Walk(dir, func(p string, i fs.FileInfo, err error) error {
			topdir := filepath.Dir(dir)
			if topdir == "." {
				topdir = ""
			}
			return c.walker(len(topdir), p, i, err, outFd)
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *car) archive(paths []string, outFile string) error {
	var err error
	outFd := os.Stdout
	c.infoFd = os.Stderr

	if outFile != "" {
		outFd, err = os.Create(outFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error creating output file", outFile, err)
			return err
		}
		defer outFd.Close()

		c.infoFd = os.Stdout
	}

	_, err = outFd.Seek(0, io.SeekCurrent)
	if err == nil {
		c.seekable = true
	} else {
		fmt.Fprintln(os.Stderr, "Warning: archive is not seekable, padding will be disabled")
	}

	err = c.walkPaths(paths, outFd)
	if err != nil {
		return err
	}

	_, err = outFd.Write([]byte(cowEnd))
	if err != nil {
		return err
	}

	if c.seekable {
		offset, err := outFd.Seek(0, io.SeekEnd)
		if err != nil {
			return err
		}
		err = outFd.Truncate(int64(round4k(uint64(offset))))
	} else {
		// As we don't know the cursor position, write the maximum alignment
		buf := make([]byte, cowAlignment)
		_, err = outFd.Write(buf)
	}

	return err
}
