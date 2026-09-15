package main

import (
	"strings"
	"testing"
)

// 验证示例配置拒绝未知字段、无效角色和缺失目标，调用端无需监听地址。
func TestDecodeConfig(t *testing.T) {
	valid := `{"role":"caller","log_directory":"./log/caller","target":"b","nodes":{"b":"127.0.0.1:19091"}}`
	if _, err := decodeConfig(strings.NewReader(valid)); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{`{}`, `{"role":"other"}`, valid + ` {}`, strings.Replace(valid, `"role"`, `"typo"`, 1)} {
		if _, err := decodeConfig(strings.NewReader(input)); err == nil {
			t.Fatalf("accepted invalid config: %s", input)
		}
	}
}
