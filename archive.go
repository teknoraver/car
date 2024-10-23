package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

func (c *car) copyTrail(in *os.File, out *os.File) error {
	written, err := io.Copy(out, in)
	if err != nil {
		return err
	}

	if written&cowMask != 0 {
		_, err = out.Write(zeroes[:cowAlignment-written])
		if err != nil {
			return err
		}
	}

	return nil
}

/*
func (c *car) reflink(entry *entry, outFile *os.File) error {
	in, err := os.Open(entry.localName)
	defer in.Close()

	if err != nil {
		return err
	}

	var offset uint64

	if entry.size >= cowAlignment {
		fcrange := unix.FileCloneRange{
			Src_fd:      int64(in.Fd()),
			Src_offset:  0,
			Src_length:  entry.size & ^uint64(cowMask),
			Dest_offset: offset,
		}

		err := unix.IoctlFileCloneRange(int(outFile.Fd()), &fcrange)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error reflinking", entry.name, err)
			return err
		}

		_, err = in.Seek(int64(fcrange.Src_length), io.SeekStart)
		if err != nil {
			return err
		}

		_, err = outFile.Seek(0, io.SeekEnd)
		if err != nil {
			return err
		}
	}

	if entry.size&cowMask != 0 {
		return c.copyTrail(in, outFile)
	}

	return nil
}
*/

func writeData(out *os.File, e entry) error {
	tag := tag{
		Kind:   tagData,
		Length: 8,
	}

	err := binary.Write(out, binary.BigEndian, &tag)
	if err != nil {
		return err
	}

	err = binary.Write(out, binary.BigEndian, e.size)
	if err != nil {
		return err
	}

	if e.Mode&unix.S_IFMT == unix.S_IFREG && e.size > 0 {
		in, err := os.Open(e.localName)
		if err != nil {
			return err
		}
		defer in.Close()

		_, err = io.Copy(out, in)
	}

	return err
}

func (c *car) writeName(out *os.File, e entry) error {
	tag := tag{
		Kind:   tagName,
		Length: uint16(len(e.name)),
	}

	err := binary.Write(out, binary.BigEndian, tag)
	if err != nil {
		return err
	}

	_, err = out.Write([]byte(e.name))

	return err
}

func (c *car) writeLink(out *os.File, e entry) error {
	tag := tag{
		Kind:   tagLinkTarget,
		Length: uint16(len(e.link)),
	}

	err := binary.Write(out, binary.BigEndian, tag)
	if err != nil {
		return err
	}

	_, err = out.Write([]byte(e.link))

	return err
}

func (c *car) writeHeader(out *os.File, e entry) error {
	_, err := out.Write([]byte(cowMagic))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Write error:", err)
		return err
	}

	tag := tag{
		Kind:   tagHeader,
		Length: uint16(binary.Size(e.fixedData)),
	}

	err = binary.Write(out, binary.BigEndian, &tag)
	if err != nil {
		return err
	}

	err = binary.Write(out, binary.BigEndian, &e.fixedData)
	if err != nil {
		return err
	}

	err = c.writeName(out, e)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Write error:", err)
		return err
	}

	if e.Mode&unix.S_IFMT == unix.S_IFLNK {
		err = c.writeLink(out, e)
		if err != nil {
			return err
		}
	}

	return writeData(out, e)
}

func unixMode(mode fs.FileMode) uint32 {
	var unixMode uint32

	switch {
	case mode&fs.ModeNamedPipe != 0:
		unixMode = unix.S_IFIFO
	case mode&fs.ModeCharDevice != 0:
		unixMode = unix.S_IFCHR
	case mode.IsDir():
		unixMode = unix.S_IFDIR
	case mode&fs.ModeDevice != 0:
		unixMode = unix.S_IFBLK
	case mode.IsRegular():
		unixMode = unix.S_IFREG
	case mode&fs.ModeSymlink != 0:
		unixMode = unix.S_IFLNK
	case mode&fs.ModeSocket != 0:
		unixMode = unix.S_IFSOCK
	}

	return unixMode | uint32(mode.Perm())
}

func (c *car) walker(strip int, p string, statinfo fs.FileInfo, err error, outFd *os.File) error {
	if c.verbose {
		fmt.Println(p)
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
		entry.dev = uint32(sys.Dev)
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
	outFd, err := os.Create(outFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error creating output file", outFile, err)
		return err
	}
	defer outFd.Close()

	return c.walkPaths(paths, outFd)
}
