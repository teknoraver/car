package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

var zeroes = make([]byte, cowAlignment)

func walker(header *header, p string, info fs.FileInfo, err error) error {
	var extradata int

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error walking", p, err)
		return err
	}

	entry := entry{
		metadata: metadata{
			mode:       uint32(info.Mode()),
			nameLength: uint16(len(p)),
		},
		name: p,
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
		entry.metadata.size = uint64(info.Size())
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

	header.entries = append(header.entries, entry)
	header.size += uint64(entrySize+entry.metadata.nameLength) + uint64(extradata)

	if *verbose {
		fmt.Println(p)
	}

	return nil
}

func genHeader(paths []string) *header {
	header := header{
		size: 4,
	}

	for _, dir := range paths {
		filepath.Walk(dir, func(p string, i fs.FileInfo, err error) error {
			return walker(&header, p, i, err)
		})
	}

	// Trailing EOR
	header.size += entrySize

	return &header
}

func writeHeader(paths []string, outFd *os.File) (*header, error) {
	header := genHeader(paths)
	var padding uint64

	padding = cowAlignment - (header.size & cowMask)
	header.size = round4k(header.size)
	curpos := header.size

	out := bufio.NewWriter(outFd)

	_, err := out.Write([]byte(cowMagic))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Write error:", err)
		return nil, err
	}

	for i := range header.entries {
		var extradata int

		if fs.FileMode(header.entries[i].metadata.mode).IsRegular() {
			header.entries[i].metadata.offset = curpos
		}
		err = binary.Write(out, binary.BigEndian, header.entries[i].metadata)
		if err != nil {
			return nil, err
		}

		_, err = out.WriteString(header.entries[i].name)
		if err != nil {
			return nil, err
		}

		if fs.FileMode(header.entries[i].metadata.mode)&fs.ModeSymlink != 0 {
			err = binary.Write(out, binary.BigEndian, header.entries[i].linkLen)
			if err != nil {
				return nil, err
			}

			_, err = out.WriteString(header.entries[i].link)
			if err != nil {
				return nil, err
			}

			extradata = 2 + int(header.entries[i].linkLen)
		}

		curpos += header.entries[i].metadata.size + uint64(extradata)
		curpos = round4k(curpos)
	}
	eor := metadata{
		mode: EOR,
	}
	err = binary.Write(out, binary.BigEndian, eor)
	if err != nil {
		return nil, err
	}

	_, err = out.Write(zeroes[:padding])
	if err != nil {
		return nil, err
	}

	out.Flush()

	return header, nil
}

func copyTrail(in *os.File, out *os.File) error {
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

func reflink(entry *entry, outFile *os.File) error {
	in, err := os.Open(entry.name)
	defer in.Close()

	if err != nil {
		return err
	}

	if entry.metadata.size != 0 && entry.metadata.size >= cowAlignment {
		fcrange := unix.FileCloneRange{
			Src_fd:      int64(in.Fd()),
			Src_offset:  0,
			Src_length:  entry.metadata.size & ^uint64(cowMask),
			Dest_offset: entry.metadata.offset,
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

	return copyTrail(in, outFile)
}

func archive(paths []string, outFile string) error {
	outFd, err := os.Create(outFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error creating output file", outFile, err)
		return err
	}
	defer outFd.Close()

	header, err := writeHeader(paths, outFd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error writing header", err)
		return err
	}

	for _, entry := range header.entries {
		if fs.FileMode(entry.metadata.mode).IsRegular() {
			err = reflink(&entry, outFd)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
