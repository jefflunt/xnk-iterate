//go:build !darwin

package ui

// PickFolder is a cross-platform fallback stub for non-macOS builds.
func PickFolder(initialDir string) (string, bool) {
	return "", false
}
