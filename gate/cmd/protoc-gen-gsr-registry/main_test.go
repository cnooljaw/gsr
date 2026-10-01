package main

import "testing"

// TestCatalogOutputPathSupportsBusinessModules 验证业务项目可把目录生成到自己的 Go module，而不依赖 GSR 路径。
func TestCatalogOutputPathSupportsBusinessModules(t *testing.T) {
	protoPath := "game/room/v1/room.proto"
	importPath := "example.com/game/protocol/room/v1"
	got, err := catalogOutputPath(protoPath, importPath, "example.com/game")
	if err != nil || got != "protocol/room/v1/room_registry.pb.go" {
		t.Fatalf("catalogOutputPath() = %q, %v; want business module path", got, err)
	}
	got, err = catalogOutputPath(protoPath, importPath, "")
	if err != nil || got != "game/room/v1/room_registry.pb.go" {
		t.Fatalf("catalogOutputPath(no module) = %q, %v; want source-relative path", got, err)
	}
	if _, err := catalogOutputPath(protoPath, importPath, "example.com/other"); err == nil {
		t.Fatal("catalogOutputPath() accepted import outside module")
	}
}
