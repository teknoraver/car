package main

const cowAlignment = 4096
const cowMask = cowAlignment - 1
const cowMagic = "CAR!"
const entrySize = 4 + 8 + 8 + 2
const EOR = 0xffffffff

var zeroes = make([]byte, cowAlignment)

type fixedData struct {
	mode       uint32
	offset     uint64
	size       uint64
	nameLength uint16
}

type entry struct {
	fixedData
	name      string
	localName string
	link      string
	linkLen   uint16
}

type header struct {
	entries []*entry
	size    uint64
}

type archive interface {
	archive(paths []string, outFile string) error
	extract(inFile string) error
}

type car struct {
	dupMap  map[uint64]*fixedData
	verbose *bool
}

func round4k(size uint64) uint64 {
	return ((size - 1) | 0xfff) + 1
}
