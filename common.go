package main

const cowAlignment = 4096
const cowMask = cowAlignment - 1
const cowMagic = "CAR!"
const entrySize = 4 + 8 + 8 + 2
const EOR = 0xffffffff

const (
	tagHeader uint16 = iota + 1
	tagName
	tagData
	tagLinkTarget
	tagDevice
	tagPadding
)

const (
	typeFifo uint16 = iota << 12
	typeChar
	typeDir
	typeBlock
	typeFile
	typeLink
	typeSocket
)

var zeroes = make([]byte, cowAlignment)

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
	Kind   uint16
	Length uint16
}

type archive interface {
	archive(paths []string, outFile string) error
	extract(inFile string) error
}

type car struct {
	verbose bool
	list    bool
}

func round4k(size uint64) uint64 {
	return (size + cowMask) & ^uint64(cowMask)
}
