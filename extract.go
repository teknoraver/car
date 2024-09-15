package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

func reflinkToFile(e *entry, inFile *os.File) error {
	out, err := os.OpenFile(e.name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fs.FileMode(e.metadata.mode).Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	if e.metadata.size == 0 {
		return nil
	}

	fcrange := unix.FileCloneRange{
		Src_fd:     int64(inFile.Fd()),
		Src_offset: e.metadata.offset,
		Src_length: round4k(e.metadata.size),
	}

	err = unix.IoctlFileCloneRange(int(out.Fd()), &fcrange)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error reflinking", e.name, err, fcrange)
		return nil
	}

	err = unix.Ftruncate(int(out.Fd()), int64(e.metadata.size))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error truncating", e.name, err)
		return nil
	}

	return nil
}

func extract(inFile string) error {
	inFd, err := os.Open(inFile)
	if err != nil {
		return err
	}
	defer inFd.Close()

	magic := make([]byte, 4)
	inFd.Read(magic)
	if string(magic) != cowMagic {
		fmt.Fprintln(os.Stderr, "Invalid input file")
		return errors.New("Invalid input file")
	}

	for {
		var e entry
		var err error

		err = binary.Read(inFd, binary.BigEndian, &e.metadata.mode)
		if err != nil {
			return err
		}
		if e.metadata.mode == 0xffffffff {
			break
		}

		err = binary.Read(inFd, binary.BigEndian, &e.metadata.offset)
		if err != nil {
			return err
		}

		err = binary.Read(inFd, binary.BigEndian, &e.metadata.size)
		if err != nil {
			return err
		}

		err = binary.Read(inFd, binary.BigEndian, &e.metadata.nameLength)
		if err != nil {
			return err
		}

		nameb := make([]byte, e.metadata.nameLength)
		_, err = inFd.Read(nameb)
		if err != nil {
			return err
		}

		e.name = string(nameb)

		switch {
		case fs.FileMode(e.metadata.mode).IsDir():
			err = os.Mkdir(e.name, 0755)
			if err != nil {
				return err
			}
			continue

		case fs.FileMode(e.metadata.mode)&fs.ModeSymlink != 0:
			var linkLen uint16
			var link string
			err = binary.Read(inFd, binary.BigEndian, &linkLen)
			if err != nil {
				return err
			}

			linkb := make([]byte, linkLen)
			_, err = inFd.Read(linkb)
			if err != nil {
				return err
			}

			link = string(linkb)
			err = os.Symlink(link, e.name)
			if err != nil {
				return err
			}

		case fs.FileMode(e.metadata.mode).IsRegular():
			err = reflinkToFile(&e, inFd)
			if err != nil {
				return err
			}
		}

		if *verbose {
			fmt.Println(e.name)
		}
	}

	return nil
}
