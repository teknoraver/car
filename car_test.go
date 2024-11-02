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
	mode    fs.FileMode
	size    uint64
	link    string
	content byte
}

var testEntries = []*testEntry{
	{mode: 0o755 | fs.ModeDir, size: 0, name: "dir1"},
	{mode: 0o644, size: 0, name: "dir1/empty"},
	{mode: 0o755, size: 16, name: "dir1/exe", content: 'x'},
	{mode: 0o600, size: 4300, name: "dir1/private", content: 'p'},
	{mode: 0o444, size: 8300, name: "dir1/readonly", content: 'r'},
	{mode: 0o755 | fs.ModeDir, size: 0, name: "dir2"},
	{mode: 0o644, size: 200, name: "dir2/200", content: '2'},
	{mode: 0o644, size: 4096, name: "dir2/4k", content: '4'},
	{mode: 0o644, size: 4192, name: "dir2/4k1", content: '1'},
	{mode: 0o644, size: 8300, name: "dir2/copy_of_readonly"},
	{mode: 0o755 | fs.ModeDir, size: 0, name: "dir2/subdir"},
	{mode: 0o777 | fs.ModeSymlink, size: 0, name: "dir2/subdir/link", link: "../4k"},
	{mode: 0o644, size: 512, name: "toplevel", content: 't'},
}

func fillFile(path string, mode fs.FileMode, c byte, size uint64) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, mode)
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
	switch {
	case e.mode.IsDir():
		if err := os.Mkdir(testDir+"/create/"+e.name, e.mode); err != nil {
			return err
		}
	case e.mode&fs.ModeSymlink != 0:
		if err := os.Symlink(e.link, testDir+"/create/"+e.name); err != nil {
			return err
		}
	default:
		if err := fillFile(testDir+"/create/"+e.name, e.mode, e.content, e.size); err != nil {
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

func testSetup(t *testing.T) error {
	var err error

	testDir = t.TempDir()
	err = os.Mkdir(testDir+"/create", 0o755)
	if err != nil {
		return err
	}

	for _, e := range testEntries {
		if err = makeEntry(e); err != nil {
			return err
		}
	}

	err = copyFile(testDir+"/create/dir1/readonly", testDir+"/create/dir2/copy_of_readonly")

	return err
}

func testCreate(t *testing.T) {
	c := car{}

	err := c.archive([]string{testDir + "/create"}, testDir+"/test.car")
	if err != nil {
		t.Fatal(err)
	}
}

func testList(t *testing.T) {
	c := car{
		list: true,
	}

	oldStdout := os.Stdout

	os.Stdout, _ = os.Open(os.DevNull)
	err := c.extract(testDir + "/test.car")
	os.Stdout = oldStdout

	if err != nil {
		t.Fatal(err)
	}
}

func testExtract(t *testing.T) {
	c := car{}

	err := c.extract(testDir + "/test.car")
	if err != nil {
		t.Fatal(err)
	}
}

func TestCar(t *testing.T) {
	err := testSetup(t)
	if err != nil {
		t.Fatal(err)
	}

	if t.Run("Create", testCreate) {
		t.Run("List", testList)
		t.Run("Extract", testExtract)
	}
}
