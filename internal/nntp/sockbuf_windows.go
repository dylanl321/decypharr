//go:build windows

package nntp

func setSocketBuffers(_ uintptr, _, _ int) error {
	return nil
}
