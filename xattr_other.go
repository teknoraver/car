//go:build !linux

package main

func listXattrs(string) ([]xattr, error) {
	return nil, nil
}

func setXattr(string, string, []byte) error {
	return nil
}
