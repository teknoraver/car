//go:build linux

package main

import "golang.org/x/sys/unix"

/* setTimes sets the access and modification time of path to mtime,
 * without following symlinks, so the link itself is updated. */
func setTimes(path string, mtime int64) error {
	ts := []unix.Timespec{
		unix.NsecToTimespec(mtime),
		unix.NsecToTimespec(mtime),
	}
	return unix.UtimesNanoAt(unix.AT_FDCWD, path, ts, unix.AT_SYMLINK_NOFOLLOW)
}
