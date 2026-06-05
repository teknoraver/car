//go:build linux

package main

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestXattrRoundTrip(t *testing.T) {
	const name = "user.car_test"
	want := []byte("hello xattr")

	root := t.TempDir()

	src := root + "/src"
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}

	file := src + "/file"
	if err := os.WriteFile(file, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := unix.Lsetxattr(file, name, want, 0); err != nil {
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
			t.Skip("filesystem does not support xattrs")
		}
		t.Fatal(err)
	}

	archivePath := root + "/test.car"
	c := car{xattrs: true}
	if err := c.archive([]string{src}, archivePath); err != nil {
		t.Fatal(err)
	}

	dst := root + "/dst"
	if err := os.Mkdir(dst, 0o755); err != nil {
		t.Fatal(err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dst); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	e := car{}
	if err := e.extract(archivePath); err != nil {
		t.Fatal(err)
	}

	got, err := getList(func(buf []byte) (int, error) {
		return unix.Lgetxattr(dst+"/src/file", name, buf)
	})
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("xattr mismatch: got %q, want %q", got, want)
	}
}
