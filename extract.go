package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func prettySize(size uint64) string {
	units := "KMGTPE"
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%7d", size)
	}
	div := int64(unit)
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		units = units[1:]
	}

	if fsize := float64(size) / float64(div); fsize < 100 {
		return fmt.Sprintf("%6.1f%c", fsize, units[0])
	} else {
		return fmt.Sprintf("%6.0f%c", fsize, units[0])
	}
}

func verbosePrint(e entry) {
	// fs.FileMode.String() doesn't print the setuid or sticky bit
	buf := []byte("?rwxrwxrwx")
	extra := []byte("sst")
	var link string

	switch e.Mode & unix.S_IFMT {
	case unix.S_IFREG:
		buf[0] = '-'
	case unix.S_IFDIR:
		buf[0] = 'd'
	case unix.S_IFLNK:
		buf[0] = 'l'
		link = " -> " + e.link
	/* S_IFCHR is a subset of S_IFBLK, so order is important */
	case unix.S_IFBLK:
		buf[0] = 'b'
	case unix.S_IFCHR:
		buf[0] = 'c'
	case unix.S_IFIFO:
		buf[0] = 'p'
	case unix.S_IFSOCK:
		buf[0] = 's'
	}

	for i := range buf[1:] {
		if e.Mode&(1<<(9-i-1)) == 0 {
			buf[i+1] = '-'
		}
		if group := i / 3; i%3 == 2 {
			if e.Mode&(1<<(9-group+2)) != 0 {
				buf[i+1] = extra[group]
				if e.Mode&(1<<(9-i-1)) == 0 {
					buf[i+1] -= 'a' - 'A'
				}
			}
		}
	}

	var size string
	perm := string(buf)
	mtime := time.Unix(e.Mtime/1e9, e.Mtime%1e9).Format("2006-01-02 15:04")
	uid := strconv.FormatUint(uint64(e.Uid), 10)
	gid := strconv.FormatUint(uint64(e.Gid), 10)

	if e.Mode&unix.S_IFMT == unix.S_IFBLK || e.Mode&unix.S_IFMT == unix.S_IFCHR {
		size = fmt.Sprintf("%3d,%3d", unix.Major(uint64(e.dev)), unix.Minor(uint64(e.dev)))
	} else {
		size = prettySize(e.size)
	}

	if user, err := user.LookupId(uid); err == nil {
		uid = user.Username
	}
	if group, err := user.LookupGroupId(gid); err == nil {
		gid = group.Name
	}

	fmt.Printf("%s %12s %12s %s %s %s%s\n", perm, uid, gid, size, mtime, e.name, link)
}

func (c *car) extractFile(archive *os.File, e entry, mode fs.FileMode) error {
	f, err := os.OpenFile(e.name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer f.Close()

	if e.size == 0 {
		return nil
	}

	err = reflinkFromArchive(archive, f, e.size)
	if err != nil && errors.Is(err, reflinkError) {
		_, err = io.CopyN(f, archive, int64(e.size))
	}

	return err
}

func (c *car) extractEntry(archive *os.File, e entry) error {
	var err, reterr error

	mode := uint32(e.Mode & 0o777)
	fsFileMode := fs.FileMode(mode)

	// File could already exist and have no write permission, delete it
	if e.Mode&unix.S_IFMT != unix.S_IFDIR {
		os.Remove(e.name)
	}

	switch e.Mode & unix.S_IFMT {
	case unix.S_IFREG:
		err = c.extractFile(archive, e, fsFileMode)
	case unix.S_IFDIR:
		modeSet := fs.FileMode(e.Mode & 0o7777)
		if modeSet&0o300 != 0o300 {
			/* A directory can have no write or execute permissions, yet contain files. To correctly
			 * extract files inside, set permissions to 0300 now and defer the real permission set. */
			c.dirModes = append(c.dirModes, dirMode{e.name, modeSet})
			modeSet = 0o300
		}
		/* os.MkdirAll() never returns error on exist. If directory
		 * already exists, ignore it and just change permission later */
		err = os.Mkdir(e.name, fsFileMode|modeSet)
		if errors.Is(err, os.ErrExist) {
			err = nil
		}
	case unix.S_IFLNK:
		err = os.Symlink(e.link, e.name)
	case unix.S_IFBLK, unix.S_IFCHR:
		err = syscall.Mknod(e.name, e.Mode, int(e.dev))
	case unix.S_IFIFO:
		err = syscall.Mkfifo(e.name, mode)
	}

	if err != nil {
		return err
	}

	/* Errors past this point are not fatal, but will printed
	 * on stderr and led to error exit status */

	if c.superUser {
		/* chmod() clears the SetUID bit and xattrs, so order is important */
		err = os.Lchown(e.name, int(e.Uid), int(e.Gid))
		if err != nil {
			c.error = 1
			fmt.Fprintf(os.Stderr, "can't set owner: %v\n", err)
			/* Save the first error */
			reterr = err
		}
	}

	if e.Mode&0o7000 != 0 {
		if e.Mode&unix.S_ISUID != 0 {
			fsFileMode |= fs.ModeSetuid
		}
		if e.Mode&unix.S_ISGID != 0 {
			fsFileMode |= fs.ModeSetgid
		}
		if e.Mode&unix.S_ISVTX != 0 {
			fsFileMode |= fs.ModeSticky
		}
		err = os.Chmod(e.name, fsFileMode)
		if err != nil {
			c.error = 1
			fmt.Fprintf(os.Stderr, "can't set permissions: %v\n", err)
			if reterr == nil {
				/* Having to choose, report only the first error */
				reterr = err
			}
		}
	}

	return reterr
}

func (c *car) parseEntry(archive *os.File) (*entry, error) {
	buf := make([]byte, 4)

	_, err := archive.Read(buf[:4])
	if err != nil {
		return nil, err
	}

	if string(buf[:4]) == cowEnd {
		return nil, io.EOF
	} else if string(buf[:4]) != cowMagic {
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

		switch tag.Tag {
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
		case tagData:
			if tag.Length == 12 {
				var pd paddedData
				err = binary.Read(archive, binary.BigEndian, &pd)
				if err != nil {
					return nil, err
				}
				e.size = pd.Size
				_, err = archive.Seek(int64(pd.Padding), io.SeekCurrent)
				if err != nil {
					return nil, err
				}
			} else if tag.Length != 0 {
				return nil, errors.New("bad file size field width")
			}
			break tagLoop
		default:
			fmt.Fprintf(os.Stderr, "unknown tag: 0x%x\n", tag.Tag)
		}
	}

	if c.list && c.verbose {
		verbosePrint(e)
	} else if c.list || c.verbose {
		fmt.Println(e.name)
	}

	if c.list {
		if e.size > 0 {
			_, err = archive.Seek(int64(e.size), io.SeekCurrent)
			if err != nil {
				return nil, err
			}
		}
	} else {
		err = c.extractEntry(archive, e)
		if err != nil {
			c.error = 1
			fmt.Fprintf(os.Stderr, "cannot create %s: %v\n", e.name, err)
		}
	}

	return &e, nil
}

func (c *car) deferredPermissions() error {
	for i := len(c.dirModes) - 1; i >= 0; i-- {
		err := os.Chmod(c.dirModes[i].name, c.dirModes[i].mode)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *car) extract(file string) error {
	var err error
	archive := os.Stdin
	c.infoFd = os.Stderr

	if os.Getuid() == 0 {
		c.superUser = true
	}

	if file != "" {
		archive, err = os.Open(file)
		if err != nil {
			return err
		}
		defer archive.Close()

		c.infoFd = os.Stdout
	}

	for {
		_, err = c.parseEntry(archive)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	return c.deferredPermissions()

}
