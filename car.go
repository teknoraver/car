package main

import (
	"flag"
	"fmt"
	"os"
)

var verbose *bool

const cowAlignment = 4096
const cowMask = cowAlignment - 1
const cowMagic = "CAR!"
const entrySize = 4 + 8 + 8 + 2

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

func main() {
	var err error

	c := flag.Bool("c", false, "archive")
	x := flag.Bool("x", false, "extract")
	file := flag.String("f", "", "file")
	verbose = flag.Bool("v", false, "verbose")
	flag.Parse()

	if *c == *x {
		fmt.Fprintf(flag.CommandLine.Output(), "Either option -c or -x must be specified\n")
		os.Exit(1)
	}

	if *file == "" {
		fmt.Fprintf(flag.CommandLine.Output(), "Option -f must be specified\n")
		os.Exit(1)
	}

	if *c {
		if flag.NArg() == 0 {
			fmt.Fprintf(flag.CommandLine.Output(), "Missing path to compress\n")
			os.Exit(1)
		}
		err = archive(flag.Args(), *file)
	}

	if *x {
		err = extract(*file)
	}

	if err != nil {
		os.Exit(1)
	}
}
