//go:build !arm64 || purego

package sproto

// 编译期关闭不支持平台的整段入口，连参数切片检查也不保留。
const packWordsEnabled = false

func packWordsFast(_ []byte, _ []byte) (int, int) {
	return 0, 0
}
