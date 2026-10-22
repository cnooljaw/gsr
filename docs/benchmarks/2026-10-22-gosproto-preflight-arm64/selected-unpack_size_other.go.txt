//go:build !arm64 || purego

package sproto

func unpackSizeFast(_ []byte, _ int) (int, error, bool) {
	return 0, nil, false
}
