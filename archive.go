package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func (c *car) walker(header *header, strip int, p string, info fs.FileInfo, err error) error {
	var extradata int

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error walking", p, err)
		return err
	}

	storedName := p[strip:]
	if storedName[0] == '/' {
		storedName = storedName[1:]
	}

	entry := entry{
		fixedData: fixedData{
			Mode:       uint32(info.Mode()),
			NameLength: uint16(len(storedName)),
		},
		name:      storedName,
		localName: p,
	}

	switch {
	case info.Mode()&fs.ModeNamedPipe != 0:
		fallthrough
	case info.Mode()&fs.ModeSocket != 0:
		fallthrough
	case info.Mode()&fs.ModeDevice != 0:
		fallthrough
	case info.Mode()&fs.ModeCharDevice != 0:
		fallthrough
	case info.Mode()&fs.ModeIrregular != 0:
		fmt.Fprintln(os.Stderr, "Skipping", p)
		return nil
	case info.Mode().IsRegular():
		entry.Size = uint64(info.Size())
	case info.Mode().IsDir():
	case info.Mode()&fs.ModeSymlink != 0:
		entry.link, err = os.Readlink(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error reading symlink", p, err)
			return nil
		}
		entry.linkLen = uint16(len(entry.link))
		extradata = 2 + int(entry.linkLen)
	}

	header.entries = append(header.entries, &entry)
	header.Size += uint64(entrySize+entry.NameLength) + uint64(extradata)

	if *c.verbose {
		fmt.Println(p)
	}

	return nil
}

func (c *car) genHeader(paths []string) *header {
	header := header{
		Size: 4,
	}

	for _, dir := range paths {
		dir = filepath.Clean(dir)
		filepath.Walk(dir, func(p string, i fs.FileInfo, err error) error {
			topdir := filepath.Dir(dir)
			if topdir == "." {
				topdir = ""
			}
			return c.walker(&header, len(topdir), p, i, err)
		})
	}

	// Trailing EOR
	header.Size += entrySize

	return &header
}

func getHash(path string) (uint64, error) {
	hash := fnv.New64a()
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	io.Copy(hash, file)

	return hash.Sum64(), nil
}

func (c *car) writeHeader(header *header, outFd *os.File) error {
	var padding uint64

	padding = cowAlignment - (header.Size & cowMask)
	header.Size = round4k(header.Size)
	curpos := header.Size

	out := bufio.NewWriter(outFd)

	_, err := out.Write([]byte(cowMagic))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Write error:", err)
		return err
	}

	for _, e := range header.entries {
		var extradata int
		var dup bool

		if fs.FileMode(e.Mode).IsRegular() && e.Size > 0 {
			var clone *fixedData
			hash, err := getHash(e.localName)
			if err != nil {
				return err
			}
			if clone, dup = c.dupMap[hash]; dup {
				e.Offset = clone.Offset
			} else {
				c.dupMap[hash] = &e.fixedData
				e.Offset = curpos
			}
		}
		err = binary.Write(out, binary.BigEndian, e.fixedData)
		if err != nil {
			return err
		}

		_, err = out.WriteString(e.name)
		if err != nil {
			return err
		}

		if fs.FileMode(e.Mode)&fs.ModeSymlink != 0 {
			err = binary.Write(out, binary.BigEndian, e.linkLen)
			if err != nil {
				return err
			}

			_, err = out.WriteString(e.link)
			if err != nil {
				return err
			}

			extradata = 2 + int(e.linkLen)
		}

		if !dup {
			curpos += e.Size + uint64(extradata)
			curpos = round4k(curpos)
		}
	}
	eor := fixedData{
		Mode: EOR,
	}
	err = binary.Write(out, binary.BigEndian, eor)
	if err != nil {
		return err
	}

	_, err = out.Write(zeroes[:padding])
	if err != nil {
		return err
	}

	out.Flush()

	return nil
}

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

func (c *car) reflink(entry *entry, outFile *os.File) error {
	in, err := os.Open(entry.localName)
	defer in.Close()

	if err != nil {
		return err
	}

	if entry.Size >= cowAlignment {
		fcrange := unix.FileCloneRange{
			Src_fd:      int64(in.Fd()),
			Src_offset:  0,
			Src_length:  entry.Size & ^uint64(cowMask),
			Dest_offset: entry.Offset,
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

	if entry.Size&cowMask != 0 {
		return c.copyTrail(in, outFile)
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

	header := c.genHeader(paths)

	err = c.writeHeader(header, outFd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error writing header", err)
		return err
	}

	var hashes map[uint64]struct{}
	for _, entry := range header.entries {
		if fs.FileMode(entry.Mode).IsRegular() && entry.Size > 0 {
			if _, seen := hashes[entry.hash]; !seen {
				err = c.reflink(entry, outFd)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}
