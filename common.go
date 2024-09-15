package main

var verbose *bool

const cowAlignment = 4096
const cowMask = cowAlignment - 1
const cowMagic = "CAR!"
const entrySize = 4 + 8 + 8 + 2
const EOR = 0xffffffff

type metadata struct {
	mode       uint32
	offset     uint64
	size       uint64
	nameLength uint16
}

type entry struct {
	metadata metadata
	name     string
	link     string
	linkLen  uint16
}

type header struct {
	entries []entry
	size    uint64
}

func round4k(size uint64) uint64 {
	return ((size - 1) | 0xfff) + 1
}
