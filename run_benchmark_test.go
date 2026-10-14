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
				node := &Node{
					names:     make(map[cluster.ServiceName]runtime.ServiceID, count),
					namesByID: make(map[runtime.ServiceID][]cluster.ServiceName, count),
					active:    make(map[runtime.ServiceID]struct{}, count),
				}
				for i := range count {
					id := runtime.ServiceID(i + 1)
					node.activateID(id)
					if err := node.RegisterService(cluster.ServiceName(strconv.Itoa(i)), id); err != nil {
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

// BenchmarkServiceNameResolve 衡量大量名称下串行和并行解析 ServiceID 的成本。
func BenchmarkServiceNameResolve(b *testing.B) {
	for _, count := range []int{100, 10000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			node := &Node{
				names:     make(map[cluster.ServiceName]runtime.ServiceID, count),
				namesByID: make(map[runtime.ServiceID][]cluster.ServiceName, count),
				active:    make(map[runtime.ServiceID]struct{}, count),
			}
			names := make([]cluster.ServiceName, count)
			for i := range count {
				id := runtime.ServiceID(i + 1)
				names[i] = cluster.ServiceName(strconv.Itoa(i))
				node.activateID(id)
				if err := node.RegisterService(names[i], id); err != nil {
					b.Fatal(err)
				}
			}
			b.Run("serial", func(b *testing.B) {
				b.ReportAllocs()
				for i := range b.N {
					if _, err := node.resolve(names[i%count]); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("parallel", func(b *testing.B) {
				b.ReportAllocs()
				b.RunParallel(func(pb *testing.PB) {
					i := 0
					for pb.Next() {
						if _, err := node.resolve(names[i%count]); err != nil {
							b.Fatal(err)
						}
						i++
					}
				})
			})
		})
	}
}
