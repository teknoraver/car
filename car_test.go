package main

import (
	"bufio"
	"io/fs"
	"os"
	"testing"
)

var testDir string

var rightHeader = header{
	size: 406,
	entries: []*entry{
		{fixedData: fixedData{mode: 020000000755, size: 0}, name: "dir1"},
		{fixedData: fixedData{mode: 0644, size: 0}, name: "dir1/empty"},
		{fixedData: fixedData{mode: 0755, size: 16}, name: "dir1/exe"},
		{fixedData: fixedData{mode: 0600, size: 4300}, name: "dir1/private"},
		{fixedData: fixedData{mode: 0444, size: 8300}, name: "dir1/readonly"},
		{fixedData: fixedData{mode: 020000000755, size: 0}, name: "dir2"},
		{fixedData: fixedData{mode: 0644, size: 200}, name: "dir2/200"},
		{fixedData: fixedData{mode: 0644, size: 4096}, name: "dir2/4k"},
		{fixedData: fixedData{mode: 0644, size: 4192}, name: "dir2/4k1"},
		{fixedData: fixedData{mode: 020000000755, size: 0}, name: "dir2/subdir"},
		{fixedData: fixedData{mode: 01000000777, size: 0}, name: "dir2/subdir/link", link: "../4k"},
		{fixedData: fixedData{mode: 0644, size: 512}, name: "toplevel"},
	},
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

func makeEntry(e *entry, c byte) error {
	if fs.FileMode(e.mode)&fs.ModeDir != 0 {
		if err := os.Mkdir(testDir+"/"+e.name, os.FileMode(e.mode)); err != nil {
			return err
		}
	} else if fs.FileMode(e.mode)&fs.ModeSymlink != 0 {
		if err := os.Symlink(e.link, testDir+"/"+e.name); err != nil {
			return err
		}
	} else {
		if err := fillFile(testDir+"/"+e.name, int(e.mode), c, e.size); err != nil {
			return err
		}
	}

	return nil
}

func testSetup(t *testing.T) error {
	var err error
	verbose = new(bool)

	testDir = t.TempDir()

	for i, e := range rightHeader.entries {
		if err = makeEntry(e, 'A'+byte(i)); err != nil {
			return err
		}
	}

	return nil
}

func TestGenHeader(t *testing.T) {
	if err := testSetup(t); err != nil {
		t.Fatal(err)
	}

	header := genHeader([]string{
		testDir + "/dir1",
		testDir + "/dir2/",
		testDir + "//toplevel"},
	)
	if header == nil {
		t.Fatal("genHeader failed")
	}

	if header.size != rightHeader.size {
		t.Error("Header size mismatch, expected", rightHeader.size, "got", header.size)
	}

	if len(header.entries) != len(rightHeader.entries) {
		t.Error("Header entry count mismatch, expected", len(rightHeader.entries), ", got", len(header.entries))
	}

	for i, v := range header.entries {
		if v.name != rightHeader.entries[i].name {
			t.Error("Entry name mismatch, expected", rightHeader.entries[i].name, "got", v.name)
		}
		if v.size != rightHeader.entries[i].size {
			t.Error("Entry size mismatch, expected", rightHeader.entries[i].size, "got", v.size)
		}
		if v.mode != rightHeader.entries[i].mode {
			t.Errorf("Entry mode mismatch, expected %o got %o", rightHeader.entries[i].mode, v.mode)
		}
	}
}
