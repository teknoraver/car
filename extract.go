package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

func prettySize(size uint64) string {
	units := "KMGTPE"
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%5d", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	if fsize := float64(size) / float64(div); fsize < 100 {
		return fmt.Sprintf("%4.1f%c", fsize, units[exp])
	} else {
		return fmt.Sprintf("%4.0f%c", fsize, units[exp])
	}
}

func verbosePrint(e entry) {
	var buf []byte = []byte("-rwxrwxrwx")
	var link string

	switch {
	case e.Mode&unix.S_IFMT == unix.S_IFREG:
		buf[0] = '-'
	case e.Mode&unix.S_IFMT == unix.S_IFLNK:
		buf[0] = 'l'
		link = " -> " + e.link
	case e.Mode&unix.S_IFMT == unix.S_IFBLK:
		buf[0] = 'b'
	case e.Mode&unix.S_IFMT == unix.S_IFCHR:
		buf[0] = 'c'
	case e.Mode&unix.S_IFMT == unix.S_IFIFO:
		buf[0] = 'p'
	case e.Mode&unix.S_IFDIR != 0:
		buf[0] = 'd'
	}

	for i, c := range buf[1:] {
		if e.Mode&(1<<uint(9-1-i)) != 0 {
			buf[i+1] = c
		} else {
			buf[i+1] = '-'
		}
	}

	perm := string(buf)
	mtime := time.Unix(e.Mtime/1e9, e.Mtime%1e9).Format("2006-01-02 15:04")
	size := prettySize(e.size)
	uid := strconv.FormatUint(uint64(e.Uid), 10)
	gid := strconv.FormatUint(uint64(e.Gid), 10)

	if user, err := user.LookupId(uid); err == nil {
		uid = user.Username
	}
	if group, err := user.LookupGroupId(gid); err == nil {
		gid = group.Name
	}

	fmt.Printf("%s %6s %6s %s %s %s%s\n", perm, uid, gid, size, mtime, e.name, link)
}

func (c *car) parseEntry(archive *os.File) (*entry, error) {
	buf := make([]byte, 4)

	_, err := archive.Read(buf[:4])
	if err != nil {
		return nil, err
	}

	if string(buf[:4]) != cowMagic {
		return nil, errors.New("bad entry")
	}

	var e entry

tagLoop:
	for {
		var tag tag

		err := binary.Read(archive, binary.BigEndian, &tag)
		if err != nil {
			return nil, err
		}

		switch tag.Kind {
		case tagHeader:
			err = binary.Read(archive, binary.BigEndian, &e.fixedData)
			if err != nil {
				return nil, err
			}
		case tagName:
			buf := make([]byte, tag.Length)
			_, err := archive.Read(buf)
			if err != nil {
				return nil, err
			}
			e.name = string(buf)
		case tagLinkTarget:
			buf := make([]byte, tag.Length)
			_, err := archive.Read(buf)
			if err != nil {
				return nil, err
			}
			e.link = string(buf)
		case tagDevice:
			err = binary.Read(archive, binary.BigEndian, &e.dev)
			if err != nil {
				return nil, err
			}
		default:
			fmt.Fprintf(os.Stderr, "unknown tag: 0x%x\n", tag.Kind)
			fallthrough
		case tagPadding:
			if tag.Length > 0 {
				_, err := archive.Seek(int64(tag.Length), io.SeekCurrent)
				if err != nil {
					return nil, err
				}
			}
		case tagData:
			if tag.Length != 8 {
				return nil, errors.New("bad file size field width")
			}

			err = binary.Read(archive, binary.BigEndian, &e.size)
			if err != nil {
				return nil, err
			}

			if e.size > 0 {
				_, err := archive.Seek(int64(e.size), io.SeekCurrent)
				if err != nil {
					return nil, err
				}
			}
			break tagLoop
		}
	}

	if c.list && c.verbose {
		verbosePrint(e)
	} else if c.list || c.verbose {
		fmt.Println(e.name)
	}

	return &e, nil
}

func (c *car) extract(file string) error {
	archive, err := os.Open(file)
	if err != nil {
		return err
	}
	defer archive.Close()

	for {
		_, err := c.parseEntry(archive)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
