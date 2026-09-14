package logging

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const fileSizeLimit int64 = 100 << 20
const historyLimit = 10

// fileWriter 只由唯一日志写入者使用，不启动任务、不支持多进程共同滚动。
type fileWriter struct {
	path  string
	file  *os.File
	size  int64
	limit int64
}

func newFileWriter(directory string) (*fileWriter, error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(directory, 0750); err != nil {
		return nil, err
	}
	writer := &fileWriter{path: filepath.Join(directory, "gsr.log"), limit: fileSizeLimit}
	if err := writer.open(); err != nil {
		return nil, err
	}
	return writer, nil
}

// regularFile 拒绝符号链接和目录，避免把滚动操作作用到其它目标。
// 日志目录必须由部署方独占；不承诺防御其它进程并发替换目录项。
func regularFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("logging: not a regular file: %s", path)
	}
	return nil
}

func (writer *fileWriter) open() error {
	if err := regularFile(writer.path); err != nil {
		return err
	}
	file, err := os.OpenFile(writer.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		return errors.Join(err, file.Close())
	}
	writer.file = file
	writer.size = info.Size()
	return nil
}

func (writer *fileWriter) Write(data []byte) (int, error) {
	if writer.file == nil {
		return 0, os.ErrClosed
	}
	if int64(len(data)) > writer.limit {
		return 0, ErrInvalidRecord
	}
	if writer.size > writer.limit-int64(len(data)) {
		if err := writer.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := writer.file.Write(data)
	writer.size += int64(n)
	return n, err
}

func (writer *fileWriter) rotate() error {
	// 操作前检查全部受管理文件；只处理固定的 10 个历史名，不扫描或清理其它文件。
	for i := 1; i <= historyLimit; i++ {
		if err := regularFile(writer.path + "." + strconv.Itoa(i)); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	if err := os.Remove(writer.path + ".10"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for i := historyLimit - 1; i >= 1; i-- {
		from := writer.path + "." + strconv.Itoa(i)
		if err := os.Rename(from, writer.path+"."+strconv.Itoa(i+1)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := regularFile(writer.path); err != nil {
		return err
	}
	if err := os.Rename(writer.path, writer.path+".1"); err != nil {
		return err
	}
	return writer.open()
}

func (writer *fileWriter) Close() error {
	if writer.file == nil {
		return nil
	}
	file := writer.file
	writer.file = nil
	return file.Close()
}
