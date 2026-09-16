package main

import (
	"strings"
	"testing"
)

// 验证配置拒绝未知字段、无效角色和缺失目标，接收端使用节点名称开放监听。
func TestDecodeConfig(t *testing.T) {
	caller := `{"role":"caller","log_directory":"./log/caller","target":"b","nodes":{"b":"127.0.0.1:19091"}}`
	server := `{"role":"server","log_directory":"./log/server","listen":"b","nodes":{"b":"127.0.0.1:19091"}}`
	for _, input := range []string{caller, server} {
		if _, err := decodeConfig(strings.NewReader(input)); err != nil {
			t.Fatal(err)
		}
	}
	for _, input := range []string{`{}`, `{"role":"other"}`, caller + ` {}`, strings.Replace(caller, `"role"`, `"typo"`, 1), strings.Replace(server, `"listen":"b"`, `"listen":"missing"`, 1)} {
		if _, err := decodeConfig(strings.NewReader(input)); err == nil {
			t.Fatalf("accepted invalid config: %s", input)
		}
	}
}
