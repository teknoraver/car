package main

import (
	"flag"
	"fmt"
	"os"
)

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
