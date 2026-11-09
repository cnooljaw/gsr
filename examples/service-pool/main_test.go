package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// 验证示例完整接通入口、独立调用者、命名池及结果收集，并正常结束宿主等待。
func TestServicePoolExample(t *testing.T) {
	var output bytes.Buffer
	if err := run(&output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("output=%q", output.String())
	}
	jobs := make(map[int]bool)
	workers := make(map[uint64]bool)
	for _, line := range lines {
		var job, completed int
		var worker uint64
		if _, err := fmt.Sscanf(line, "job=%d worker=%d completed=%d", &job, &worker, &completed); err != nil {
			t.Fatal(err)
		}
		if job < 1 || job > 2 || jobs[job] || worker == 0 || completed != 1 {
			t.Fatalf("invalid result: %s", line)
		}
		jobs[job] = true
		workers[worker] = true
	}
	if len(workers) != 2 {
		t.Fatalf("expected two independent members: %q", output.String())
	}
}
