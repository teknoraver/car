package main

import (
	"flag"
	"fmt"
	"os"
)

func b(b bool) int {
	if b {
		return 1
	}
	return 0
}

func main() {
	var err error

	t := flag.Bool("t", false, "list")
	c := flag.Bool("c", false, "archive")
	x := flag.Bool("x", false, "extract")
	file := flag.String("f", "", "file")
	verbose := flag.Bool("v", false, "verbose")
	flag.Parse()

	if b(*t)+b(*c)+b(*x) != 1 {
		fmt.Fprintln(os.Stderr, "Exactly one ption -t, -c or -x must be specified")
		os.Exit(1)
	}

	var a archive = &car{
		verbose: *verbose,
		list:    *t,
	}

	switch {
	case *c:
		if flag.NArg() == 0 {
			fmt.Fprintln(os.Stderr, "Missing path to compress")
			os.Exit(1)
		}

		err = a.archive(flag.Args(), *file)

	case *t, *x:
		err = a.extract(*file)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(a.(*car).error)
}
