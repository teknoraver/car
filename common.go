package main

const cowAlignment = 4096
const cowMask = cowAlignment - 1
const cowMagic = "CAR!"
const entrySize = 4 + 8 + 8 + 2
const EOR = 0xffffffff

var zeroes = make([]byte, cowAlignment)

type fixedData struct {
	Mode       uint32
	Offset     uint64
	Size       uint64
	NameLength uint16
}

type entry struct {
	fixedData
	name      string
	localName string
	link      string
	linkLen   uint16
	hash      uint64
}

type header struct {
	entries []*entry
	Size    uint64
}

type archive interface {
	archive(paths []string, outFile string) error
	extract(inFile string) error
}

type car struct {
	dupMap  map[uint64]*fixedData
	verbose *bool
	header  header
}

func round4k(Size uint64) uint64 {
	return ((Size - 1) | 0xfff) + 1
}
