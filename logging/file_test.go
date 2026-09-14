package logging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 验证小容量滚动保持完整记录、历史新旧顺序和保留数量，不删除无关文件。
func TestFileRotation(t *testing.T) {
	dir := t.TempDir()
	writer, err := newFileWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	writer.limit = 4
	defer writer.Close()
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 13; i++ {
		if _, err := writer.Write([]byte(fmt.Sprintf("%03d\n", i))); err != nil {
			t.Fatal(err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(dir, "gsr.log")); err != nil || string(data) != "012\n" {
		t.Fatalf("current=%q err=%v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "gsr.log.1")); err != nil || string(data) != "011\n" {
		t.Fatalf("latest=%q err=%v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "gsr.log.10")); err != nil || string(data) != "002\n" {
		t.Fatalf("oldest=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gsr.log.11")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("too many histories")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "keep.txt")); string(data) != "keep" {
		t.Fatal("unrelated file changed")
	}
	// 重新打开只追加，不清空。
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	writer, err = newFileWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err := writer.Write([]byte("tail\n")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "gsr.log"))
	if string(data) != "012\ntail\n" {
		t.Fatalf("restart=%q", data)
	}
}

// 验证 Logger 在跨文件滚动时不重置序号且每条 JSON 保持完整。
func TestLoggerRotationSequence(t *testing.T) {
	dir := t.TempDir()
	logger, err := New(Config{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	logger.core.file.limit = 350
	for i := 0; i < 5; i++ {
		if err := logger.Info("sequence", "index", i); err != nil {
			t.Fatal(err)
		}
	}
	if err := logger.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	sequence := uint64(1)
	if _, err := os.Stat(filepath.Join(dir, "gsr.log.1")); err != nil {
		t.Fatalf("rotation did not occur: %v", err)
	}
	for i := historyLimit; i >= 0; i-- {
		name := "gsr.log"
		if i > 0 {
			name = fmt.Sprintf("gsr.log.%d", i)
		}
		file, err := os.Open(filepath.Join(dir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		decoder := json.NewDecoder(file)
		for {
			var entry record
			err := decoder.Decode(&entry)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				file.Close()
				t.Fatal(err)
			}
			if entry.Sequence != sequence {
				t.Errorf("seq=%d want=%d", entry.Sequence, sequence)
			}
			sequence++
		}
		file.Close()
	}
	if sequence != 6 {
		t.Fatalf("sequence=%d", sequence)
	}
}

// 验证滚动时遇到非普通历史文件立即失败，保留现有内容并拒绝继续接纳。
func TestLoggerRotationFailure(t *testing.T) {
	dir := t.TempDir()
	logger, err := New(Config{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	logger.core.file.limit = 350
	if err := os.Mkdir(filepath.Join(dir, "gsr.log.1"), 0750); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := logger.Info("rotate"); err != nil {
			break
		}
	}
	if err := logger.Close(context.Background()); err == nil {
		t.Fatal("missing rotation error")
	}
	if logger.Err() == nil || logger.Info("late") == nil {
		t.Fatal("failure state missing")
	}
	if info, err := os.Stat(filepath.Join(dir, "gsr.log.1")); err != nil || !info.IsDir() {
		t.Fatal("obstacle deleted")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "gsr.log")); len(data) == 0 {
		t.Fatal("original log lost")
	}
}

// 验证相对目录文件输出由 Logger 关闭，冲突配置或无效路径不静默转终端。
func TestLoggerDirectory(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.Rel(cwd, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	logger, err := New(Config{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Info("file"); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if logger.Err() != nil {
		t.Fatal(logger.Err())
	}
	if data, err := os.ReadFile(filepath.Join(dir, "gsr.log")); err != nil || len(data) == 0 {
		t.Fatalf("data=%q err=%v", data, err)
	}
	if _, err := New(Config{Directory: dir, Writer: io.Discard}); err == nil {
		t.Fatal("ambiguous output")
	}
	if _, err := New(Config{Directory: filepath.Join(dir, "gsr.log", "child")}); err == nil {
		t.Fatal("invalid path accepted")
	}
}

// 验证首个写入失败后不再调用 Writer，拒绝新日志并将所有已接纳残留计入失败。
func TestLoggerFailureStopsAdmission(t *testing.T) {
	writer := &blockedFailure{entered: make(chan struct{}), release: make(chan struct{})}
	logger, _ := New(Config{Writer: writer, Capacity: 4})
	if err := logger.Info("first"); err != nil {
		t.Fatal(err)
	}
	<-writer.entered
	if err := logger.Info("queued"); err != nil {
		t.Fatal(err)
	}
	close(writer.release)
	// 不调用 Close 也必须自动结束失败写入者，并允许查询运行故障。
	select {
	case <-logger.core.done:
	case <-time.After(time.Second):
		t.Fatal("failed writer did not stop")
	}
	if err := logger.Close(context.Background()); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	if !errors.Is(logger.Err(), io.ErrClosedPipe) || !errors.Is(logger.Info("late"), io.ErrClosedPipe) {
		t.Fatal("lost failure")
	}
	if writer.calls != 1 || logger.WriteFailureCount() != 2 {
		t.Fatalf("calls=%d failures=%d", writer.calls, logger.WriteFailureCount())
	}
}

type blockedFailure struct {
	entered, release chan struct{}
	calls            int
}

func (writer *blockedFailure) Write([]byte) (int, error) {
	writer.calls++
	if writer.calls == 1 {
		close(writer.entered)
		<-writer.release
	}
	return 0, io.ErrClosedPipe
}
