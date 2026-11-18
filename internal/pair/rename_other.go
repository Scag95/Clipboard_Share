//go:build !linux

package pair

func renameNoReplace(oldPath, newPath string) error {
	return renameIfAbsent(oldPath, newPath)
}
