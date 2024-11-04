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

type car struct {
	verbose   bool
	list      bool
	error     int
	infoFd    io.Writer
	superUser bool
	dirModes  []dirMode
}

var reflinkError = errors.New("reflink not supported")

func round4k(size uint64) uint64 {
	return (size + cowMask) & ^uint64(cowMask)
}
