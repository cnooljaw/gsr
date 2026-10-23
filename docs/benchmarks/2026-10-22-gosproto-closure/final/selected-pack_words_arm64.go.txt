//go:build arm64 && !purego

package sproto

// 编译期关闭不支持平台的整段入口，连参数切片检查也不保留。
const packWordsEnabled = true

// 固定16字节表项，保存按有效源字节排序的置换索引及字节数。
type packWordEntry struct{ indexes, count uint64 }

// 固定4KiB置换表在初始化后只读，未使用的索引用255产生零字节。
var packWordTable = func() (table [256]packWordEntry) {
	for header := range table {
		indexes := ^uint64(0)
		rank := 0
		for position := 0; position < 8; position++ {
			if header&(1<<position) != 0 {
				indexes &^= uint64(255) << (rank * 8)
				indexes |= uint64(position) << (rank * 8)
				rank++
			}
		}
		table[header] = packWordEntry{indexes, uint64(rank)}
	}
	return table
}()

// packWordsFast 同步压紧普通word，不保留源；范围与重叠间隔由packInto建立。
func packWordsFast(dst, src []byte) (int, int) {
	return packWordsARM64(dst, src, &packWordTable)
}

//go:noescape
func packWordsARM64(dst, src []byte, table *[256]packWordEntry) (written int, consumed int)
