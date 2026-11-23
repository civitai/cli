//go:build unix

package cmd

import "syscall"

// dirCreatable reports whether a new entry could be created inside the existing
// directory dir: access(2) with W_OK|X_OK. That answers EACCES for a directory
// the user cannot write and EROFS on a read-only filesystem — the errno the
// real MkdirAll would fail with. W_OK (2) and X_OK (1) are the POSIX values;
// package syscall does not export them on every unix.
func dirCreatable(dir string) error {
	const wOK, xOK = 0x2, 0x1
	return syscall.Access(dir, wOK|xOK)
}
