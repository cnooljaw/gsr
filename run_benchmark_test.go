package gsr

import (
	"strconv"
	"testing"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/runtime"
)

// BenchmarkServiceNameCleanup 衡量大量命名 Service 依次停止时的名称表清理成本。
func BenchmarkServiceNameCleanup(b *testing.B) {
	for _, count := range []int{100, 1000, 10000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			for range b.N {
				b.StopTimer()
				node := &application{
					names:     make(map[cluster.ServiceName]runtime.ServiceID, count),
					namesByID: make(map[runtime.ServiceID][]cluster.ServiceName, count),
					active:    make(map[runtime.ServiceID]struct{}, count),
				}
				for i := range count {
					id := runtime.ServiceID(i + 1)
					node.activateID(id)
					if err := node.register(cluster.ServiceName(strconv.Itoa(i)), id); err != nil {
						b.Fatal(err)
					}
				}
				b.StartTimer()
				for i := range count {
					node.unregisterID(runtime.ServiceID(i + 1))
				}
			}
		})
	}
}
