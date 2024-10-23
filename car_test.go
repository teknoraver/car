package main

import (
	"bufio"
	"io"
	"io/fs"
	"os"
	"testing"
)

var testDir string

type testEntry struct {
	name    string
	mode    uint32
	size    uint64
	link    string
	content byte
}

var rightHeader = []*testEntry{
	{mode: 020000000755, size: 0, name: "dir1"},
	{mode: 0644, size: 0, name: "dir1/empty"},
	{mode: 0755, size: 16, name: "dir1/exe", content: 'x'},
	{mode: 0600, size: 4300, name: "dir1/private", content: 'p'},
	{mode: 0444, size: 8300, name: "dir1/readonly", content: 'r'},
	{mode: 020000000755, size: 0, name: "dir2"},
	{mode: 0644, size: 200, name: "dir2/200", content: '2'},
	{mode: 0644, size: 4096, name: "dir2/4k", content: '4'},
	{mode: 0644, size: 4192, name: "dir2/4k1", content: '1'},
	{mode: 0644, size: 8300, name: "dir2/copy_of_readonly"},
	{mode: 020000000755, size: 0, name: "dir2/subdir"},
	{mode: 01000000777, size: 0, name: "dir2/subdir/link", link: "../4k"},
	{mode: 0644, size: 512, name: "toplevel", content: 't'},
}

func fillFile(path string, mode int, c byte, size uint64) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, os.FileMode(mode))
	if err != nil {
		return err
	}
	defer f.Close()

	f2 := bufio.NewWriter(f)
	defer f2.Flush()

	for i := uint64(0); i < size; i++ {
		if err := f2.WriteByte(c); err != nil {
			return err
		}
	}

	return nil
}

func makeEntry(e *testEntry) error {
	if fs.FileMode(e.mode)&fs.ModeDir != 0 {
		if err := os.Mkdir(testDir+"/"+e.name, os.FileMode(e.mode)); err != nil {
			return err
		}
	} else if fs.FileMode(e.mode)&fs.ModeSymlink != 0 {
		if err := os.Symlink(e.link, testDir+"/"+e.name); err != nil {
			return err
		}
	} else {
		if err := fillFile(testDir+"/"+e.name, int(e.mode), e.content, e.size); err != nil {
			return err
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	srcFd, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFd.Close()

	dstFd, err := os.Create(dst)
	if err != nil {
		return err
	}

	_, err = io.Copy(dstFd, srcFd)

	return err
}

func testSetup(t *testing.T) (car, error) {
	var err error
	var car = car{
		verbose: false,
	}

	testDir = t.TempDir()

	for _, e := range rightHeader {
		if err = makeEntry(e); err != nil {
			return car, err
		}
	}

	err = copyFile(testDir+"/dir1/readonly", testDir+"/dir2/copy_of_readonly")

	return car, err
}

func TestWriteHeader(t *testing.T) {
	c, err := testSetup(t)
	if err != nil {
		t.Fatal(err)
	}

	err = c.archive([]string{testDir}, "test.car")
	if err != nil {
		t.Fatal(err)
	}
}

func TestExtract(t *testing.T) {
	c, err := testSetup(t)
	if err != nil {
		t.Fatal(err)
	}

	err = c.extract("test.car")
	if err != nil {
		t.Fatal(err)
	}
}
