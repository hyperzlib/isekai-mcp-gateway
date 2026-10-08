// Package filebundle implements directory selection and safe bundle export.
package filebundle

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Select chooses the whole external directory, falling back only when absent.
func Select(root string, builtin fs.FS) (fs.FS, error) {
	// 仅在目录确实不存在时回退到内嵌资源；其它错误（如权限）需要暴露。
	info, err := os.Lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return builtin, nil
	}
	if err != nil {
		return nil, fmt.Errorf("bundle directory %s: %w", root, err)
	}
	// 允许根目录本身是符号链接，但最终必须指向目录。
	if info.Mode()&os.ModeSymlink != 0 {
		info, err = os.Stat(root)
		if err != nil {
			return nil, fmt.Errorf("bundle directory %s: %w", root, err)
		}
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("bundle directory %s must be a directory", root)
	}
	return os.DirFS(root), nil
}

// Validate checks that an external bundle includes every shipped file.
func Validate(files, builtin fs.FS, root string) error {
	return fs.WalkDir(builtin, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		// 内嵌包中的每个文件都必须在外部目录中以普通文件存在。
		info, err := fs.Stat(files, name)
		if err != nil {
			return fmt.Errorf("required file %s: %w", filepath.Join(root, filepath.FromSlash(name)), err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("required file %s must be a regular file", name)
		}
		_, err = fs.ReadFile(files, name)
		return err
	})
}

type ExportResult struct{ Written, Skipped int }

// Export preserves paths and refuses symlinks in any destination component.
// Deployment directories must not be concurrently modified during export.
func Export(files fs.FS, destination string, overwrite bool) (ExportResult, error) {
	var result ExportResult
	root, err := filepath.Abs(destination)
	if err != nil {
		return result, err
	}
	if err = makeDirectory(root); err != nil {
		return result, err
	}

	err = fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		// 拒绝非法路径，防止拼接后逃逸目标目录。
		if !fs.ValidPath(name) {
			return fmt.Errorf("invalid bundle path %q", name)
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		if entry.IsDir() {
			return makeDirectory(target)
		}
		if err = makeDirectory(filepath.Dir(target)); err != nil {
			return err
		}
		info, err := os.Lstat(target)
		exists := err == nil
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if exists {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("export target %s must be a regular file", target)
			}
			if !overwrite {
				result.Skipped++
				return nil
			}
		}

		content, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		// 新建用 O_EXCL 防止竞态覆盖，已存在文件仅在 overwrite 时截断。
		flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
		if exists {
			flags = os.O_WRONLY | os.O_TRUNC
		}
		f, err := os.OpenFile(target, flags, 0644)
		if err != nil {
			return fmt.Errorf("export %s: %w", target, err)
		}
		// 同时保留写入与关闭两类错误，避免丢失任一失败信号。
		_, writeErr := f.Write(content)
		closeErr := f.Close()
		if err = errors.Join(writeErr, closeErr); err != nil {
			return err
		}
		result.Written++
		return nil
	})
	return result, err
}

// makeDirectory 递归创建目录，并拒绝任何层级上的符号链接或非目录。
func makeDirectory(path string) error {
	parent := filepath.Dir(path)
	// 自顶向下逐级创建，同时防止祖先为符号链接。
	if parent != path {
		if err := makeDirectory(parent); err != nil {
			return err
		}
	}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return os.Mkdir(path, 0755)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("export directory %s must be a directory without symlinks", path)
	}
	return nil
}
