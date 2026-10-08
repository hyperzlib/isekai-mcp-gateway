package templates

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"isekai-mcp-gateway/internal/failure"
)

type watchRun struct {
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

// Watch establishes directory watches synchronously, then runs until ctx ends
// or Close is called. Only one watcher can be started per manager.
func (m *Manager) Watch(ctx context.Context) error {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()
	if m.watch != nil {
		return errors.New("template watcher already started")
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	// 监听父目录以捕获模板根目录被整体替换的 rename 事件。
	parent := filepath.Dir(m.root)
	if err = w.Add(parent); err != nil {
		w.Close()
		return fmt.Errorf("watch template parent %s: %w", parent, err)
	}
	// Windows 需要额外的递归子树监听，其它平台返回空通道。
	extraEvents, extraErrors, closeExtra, err := recursiveEvents(parent)
	if err != nil {
		w.Close()
		return fmt.Errorf("watch template subtree: %w", err)
	}
	if err = syncDirectories(w, m.root); err != nil {
		closeExtra()
		w.Close()
		return err
	}
	// Close the gap between initial loading and installing watches.
	if err = m.Reload(); err != nil {
		closeExtra()
		w.Close()
		return err
	}
	run := &watchRun{stop: make(chan struct{}), done: make(chan struct{})}
	m.watch = run
	go func() {
		defer close(run.done)
		defer w.Close()
		defer closeExtra()
		var timer *time.Timer
		var tick <-chan time.Time
		defer func() {
			if timer != nil {
				timer.Stop()
			}
		}()

		handleEvent := func(event fsnotify.Event) {
			// 只关心模板根目录内、且与文件变更相关的操作。
			if !within(m.root, event.Name) {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename|fsnotify.Remove) == 0 {
				return
			}
			// A renamed directory can be replaced at the same path before this
			// event is handled. Drop watches tied to the previous inode first.
			if event.Op&(fsnotify.Rename|fsnotify.Remove) != 0 {
				for _, path := range w.WatchList() {
					if within(event.Name, path) {
						if err := w.Remove(path); err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
							failure.Log(context.Background(), "templates.background", failure.Wrap("templates.background", err))
						}
					}
				}
			}
			if err := syncDirectories(w, m.root); err != nil {
				failure.Log(context.Background(), "templates.background", failure.Wrap("templates.background", err))
			}
			// 去抖：连续事件在 200ms 静默后才触发一次重载。
			if timer == nil {
				timer = time.NewTimer(200 * time.Millisecond)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(200 * time.Millisecond)
			}
			tick = timer.C
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-run.stop:
				return
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				handleEvent(event)
			case event, ok := <-extraEvents:
				// 通道关闭后置空，避免在 select 上持续紧循环。
				if !ok {
					extraEvents = nil
					continue
				}
				handleEvent(event)
			case err, ok := <-extraErrors:
				if !ok {
					extraErrors = nil
					continue
				}
				failure.Log(context.Background(), "templates.background", failure.Wrap("templates.background", err))
			case <-tick:
				// 去抖窗口结束后同步目录监听并尝试重载。
				tick = nil
				if err := syncDirectories(w, m.root); err != nil {
					failure.Log(context.Background(), "templates.background", failure.Wrap("templates.background", err))
					continue
				}
				if err := m.Reload(); err == nil {
					slog.Info("templates reloaded")
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				failure.Log(context.Background(), "templates.background", failure.Wrap("templates.background", err))
			}
		}
	}()
	return nil
}

func (m *Manager) Close() error {
	m.watchMu.Lock()
	run := m.watch
	m.watchMu.Unlock()
	if run != nil {
		// once 保证重复 Close 安全；等待后台 goroutine 退出后再返回。
		run.once.Do(func() { close(run.stop) })
		<-run.done
	}
	return nil
}

// within 判断 name 是否位于 root 目录内，排除父目录引用。
func within(root, name string) bool {
	rel, err := filepath.Rel(root, name)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func syncDirectories(w *fsnotify.Watcher, root string) error {
	// Windows uses recursive parent notifications without descendant handles.
	if runtime.GOOS == "windows" {
		return nil
	}
	desired := map[string]bool{filepath.Dir(root): true}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == root && errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			desired[path] = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	// 对比期望目录集合与实际监听列表，增删差异项。
	active := make(map[string]bool)
	for _, path := range w.WatchList() {
		if !desired[path] {
			if err := w.Remove(path); err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
				return err
			}
		} else {
			active[path] = true
		}
	}
	for path := range desired {
		if !active[path] {
			// 目录可能在遍历与添加之间被删除，忽略此种竞态。
			if err := w.Add(path); err != nil {
				if errors.Is(err, os.ErrNotExist) && path != filepath.Dir(root) {
					continue
				}
				return fmt.Errorf("watch %s: %w", path, err)
			}
		}
	}
	return nil
}
