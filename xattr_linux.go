//go:build linux

package main

import (
	"errors"

	"golang.org/x/sys/unix"
)

/* getList wraps a two-call size probe (empty buffer to learn the size, then the
 * real read) retrying on ERANGE in case the attribute set grew in between. */
func getList(probe func([]byte) (int, error)) ([]byte, error) {
	for {
		size, err := probe(nil)
		if err != nil {
			return nil, err
		}
		if size == 0 {
			return nil, nil
		}

		buf := make([]byte, size)
		n, err := probe(buf)
		if errors.Is(err, unix.ERANGE) {
			/* It grew between the two calls, probe the size again */
			continue
		}
		if err != nil {
			return nil, err
		}
		return buf[:n], nil
	}
}

/* listXattrs returns all the extended attributes of path, without following
 * symlinks. A filesystem with no xattr support is not an error. */
func listXattrs(path string) ([]xattr, error) {
	list, err := getList(func(buf []byte) (int, error) {
		return unix.Llistxattr(path, buf)
	})
	if err != nil {
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
			return nil, nil
		}
		return nil, err
	}

	var xattrs []xattr
	for _, name := range splitNames(list) {
		value, err := getList(func(buf []byte) (int, error) {
			return unix.Lgetxattr(path, name, buf)
		})
		if err != nil {
			/* The attribute may have vanished since listing, skip it */
			if errors.Is(err, unix.ENODATA) {
				continue
			}
			return nil, err
		}
		xattrs = append(xattrs, xattr{name, value})
	}

	return xattrs, nil
}

/* setXattr restores a single extended attribute, without following symlinks. */
func setXattr(path, name string, value []byte) error {
	return unix.Lsetxattr(path, name, value, 0)
}

/* splitNames splits the NUL separated, NUL terminated name list returned by
 * listxattr into individual names. */
func splitNames(list []byte) []string {
	var names []string
	start := 0
	for i, b := range list {
		if b == 0 {
			if i > start {
				names = append(names, string(list[start:i]))
			}
			start = i + 1
		}
	}
	return names
}
