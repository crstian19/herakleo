//go:build !unix

package player

import "io/fs"

// Outside Unix the temp dir is already per-user (%TEMP% on Windows).
func ownedByMe(fs.FileInfo) bool { return true }
