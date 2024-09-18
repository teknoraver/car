package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
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

var headerSize = uint64(406)
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

func testSetup(t *testing.T) (car, error) {
	var err error
	var car = car{
		dupMap:  map[uint64]*fixedData{},
		verbose: new(bool),
	}

	testDir = t.TempDir()

	for _, e := range rightHeader {
		if err = makeEntry(e); err != nil {
			return car, err
		}
	}

	return car, nil
}

func TestGenHeader(t *testing.T) {
	c, err := testSetup(t)
	if err != nil {
		t.Fatal(err)
	}

	header := c.genHeader([]string{
		testDir + "/dir1",
		testDir + "/dir2/",
		testDir + "//toplevel"},
	)
	if header == nil {
		t.Fatal("genHeader failed")
	}

	if header.Size != headerSize {
		t.Error("Header size mismatch, expected", headerSize, "got", header.Size)
	}

	if len(header.entries) != len(rightHeader) {
		t.Error("Header entry count mismatch, expected", len(rightHeader), ", got", len(header.entries))
	}

	for i, v := range header.entries {
		if v.name != rightHeader[i].name {
			t.Error("Entry name mismatch, expected", rightHeader[i].name, "got", v.name)
		}
		if v.Size != rightHeader[i].size {
			t.Error("Entry size mismatch, expected", rightHeader[i].size, "got", v.Size)
		}
		if v.Mode != rightHeader[i].mode {
			t.Errorf("Entry mode mismatch, expected %o got %o", rightHeader[i].mode, v.Mode)
		}
	}
}

func parseHeader(path string) error {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return err
	}

	if fileInfo.Size() != int64(0x1000) {
		return fmt.Errorf("Header size mismatch, expected 4kb got %d", fileInfo.Size())
	}

	headerFd, err := os.Open(path)
	if err != nil {
		return err
	}
	defer headerFd.Close()

	headerReader := bufio.NewReader(headerFd)

	buf := make([]byte, 0x1000)
	if _, err = headerReader.Read(buf[:4]); err != nil {
		return err
	}
	if magic := string(buf[:4]); magic != cowMagic {
		return fmt.Errorf("Header magic mismatch, expected '%s' got '%s'", cowMagic, magic)
	}

	for i := 0; ; i++ {
		var entry entry
		if err = binary.Read(headerReader, binary.BigEndian, &entry.fixedData); err != nil {
			return err
		}
		if entry.Mode == EOR {
			break
		}

		if i >= len(rightHeader) {
			return fmt.Errorf("Header entry count mismatch, expected %d got %d", len(rightHeader), i)
		}

		_, err = headerReader.Read(buf[:entry.NameLength])
		if err != nil {
			return err
		}

		name := string(buf[:entry.NameLength])
		e := rightHeader[i]

		if entry.Mode != e.mode {
			return fmt.Errorf("Entry mode mismatch, expected %o got %o", e.mode, entry.Mode)
		}
		if name != e.name {
			return fmt.Errorf("Entry name mismatch, expected %s got %s", e.name, name)
		}
		if entry.Size != e.size {
			return fmt.Errorf("Entry size mismatch, expected %d got %d", e.size, entry.Size)
		}

		if fs.FileMode(entry.Mode)&fs.ModeSymlink != 0 {
			err = binary.Read(headerReader, binary.BigEndian, &entry.linkLen)
			if err != nil {
				return err
			}
			_, err = headerReader.Read(buf[:entry.linkLen])
			if err != nil {
				return err
			}
			if link := string(buf[:entry.linkLen]); link != e.link {
				return fmt.Errorf("Entry link mismatch, expected %s got %s", e.link, link)
			}
		}
	}
	n, err := headerReader.Read(buf)
	if err != nil {
		return err
	}

	for _, v := range buf[:n] {
		if v != 0 {
			return fmt.Errorf("Garbage data found in header padding: %x", v)
		}
	}

	return nil
}

func TestWriteHeader(t *testing.T) {
	c, err := testSetup(t)
	if err != nil {
		t.Fatal(err)
	}

	header := c.genHeader([]string{
		testDir + "/dir1",
		testDir + "/dir2",
		testDir + "/toplevel"},
	)
	if header == nil {
		t.Fatal("genHeader failed")
	}

	outDir := t.TempDir()
	outDir = "."
	outFd, err := os.Create(outDir + "/test.car")
	if err != nil {
		t.Fatal(err)
	}
	defer outFd.Close()

	if err = c.writeHeader(header, outFd); err != nil {
		t.Fatal(err)
	}
	outFd.Close()

	if err = parseHeader(outDir + "/test.car"); err != nil {
		t.Fatal(err)
	}
}
