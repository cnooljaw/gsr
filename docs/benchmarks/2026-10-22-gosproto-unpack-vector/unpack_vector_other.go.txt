//go:build !arm64 || purego

package sproto

func unpackVector(_, _ []byte) bool { return false }
