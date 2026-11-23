//go:build !unix

package cmd

// dirCreatable has no access(2) equivalent here, so only checkOutputDir's
// "a path component is a regular file" half applies on this platform.
func dirCreatable(string) error { return nil }
