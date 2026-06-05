package main

import (
	"errors"
	"io"
	"io/fs"
)

const cowAlignment = 4096
const cowMask = cowAlignment - 1
const cowMagic = "CAR!"
const cowEnd = "!RAC"

const (
	tagHeader uint16 = iota + 1
	tagName
	tagData
	tagLinkTarget
	tagDevice
	tagXattr
)

type fixedData struct {
	Mode  uint32
	Uid   uint32
	Gid   uint32
	Mtime int64
}

type entry struct {
	fixedData
	name      string
	size      uint64
	localName string
	link      string
	dev       uint32
	xattrs    []xattr
}

/*
An extended attribute. On Linux this also carries POSIX ACLs
(system.posix_acl_*) and security attributes (security.selinux,
security.capability), as they are all exposed as xattrs.
*/
type xattr struct {
	name  string
	value []byte
}

/*
Just a TLV (Type, Length, Value) structure,
but "type" is a reserved word in Go
*/
type tag struct {
	Tag    uint16
	Length uint16
}

type paddedData struct {
	Size    uint64
	Padding uint32
}

type archive interface {
	archive(paths []string, outFile string) error
	extract(inFile string) error
}

type dirMode struct {
	name string
	mode fs.FileMode
}

type dirTime struct {
	name  string
	mtime int64
}

type car struct {
	verbose   bool
	list      bool
	xattrs    bool
	error     int
	infoFd    io.Writer
	superUser bool
	dirModes  []dirMode
	dirTimes  []dirTime
	destDir   string
	seekable  bool
}

var reflinkError = errors.New("reflink not supported")

func round4k(size uint64) uint64 {
	return (size + cowMask) & ^uint64(cowMask)
}
