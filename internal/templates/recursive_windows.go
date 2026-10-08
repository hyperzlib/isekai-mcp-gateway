package templates

import (
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/windows"
)

// Windows prevents renaming a directory when handles to its descendants are
// open. Observe the entire subtree through one parent handle instead.
func recursiveEvents(parent string) (<-chan fsnotify.Event, <-chan error, func(), error) {
	path, err := windows.UTF16PtrFromString(parent)
	if err != nil {
		return nil, nil, nil, err
	}
	// FILE_FLAG_BACKUP_SEMANTICS 允许打开目录，OVERLAPPED 支持异步通知。
	h, err := windows.CreateFile(path, windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, nil, nil, err
	}
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(h)
		return nil, nil, nil, err
	}
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(event)
		windows.CloseHandle(h)
		return nil, nil, nil, err
	}
	events := make(chan fsnotify.Event, 128)
	errorsOut := make(chan error, 1)
	done := make(chan struct{})
	ready := make(chan error, 1)
	stopping := make(chan struct{})
	var once sync.Once
	// closeFn 触发停止事件并等待后台 goroutine 完全退出。
	closeFn := func() { once.Do(func() { close(stopping); windows.SetEvent(stop) }); <-done }
	go func() {
		defer close(done)
		defer close(events)
		defer close(errorsOut)
		defer windows.CloseHandle(h)
		defer windows.CloseHandle(event)
		defer windows.CloseHandle(stop)

		// 错误上报在停止时让路，避免 goroutine 卡死在发送上。
		report := func(err error) {
			select {
			case errorsOut <- err:
			case <-stopping:
			}
		}
		buf := make([]byte, 64*1024)
		initial := true

		for {
			windows.ResetEvent(event)
			ov := windows.Overlapped{HEvent: event}
			// 监听子目录递归变化，覆盖重命名、写入与大小变化。
			filter := uint32(windows.FILE_NOTIFY_CHANGE_FILE_NAME | windows.FILE_NOTIFY_CHANGE_DIR_NAME | windows.FILE_NOTIFY_CHANGE_LAST_WRITE | windows.FILE_NOTIFY_CHANGE_SIZE)
			err := windows.ReadDirectoryChanges(h, &buf[0], uint32(len(buf)), true, filter, nil, &ov, 0)
			// 重叠 IO 返回 ERROR_IO_PENDING 表示已挂起，属正常路径。
			if err != nil && !errors.Is(err, windows.ERROR_IO_PENDING) {
				if initial {
					ready <- err
				}
				report(err)
				return
			}
			if initial {
				ready <- nil
				initial = false
			}
			// WAIT_OBJECT_0 对应停止事件，需取消挂起的读取。
			wait, err := windows.WaitForMultipleObjects([]windows.Handle{stop, event}, false, windows.INFINITE)
			if err != nil || wait == windows.WAIT_OBJECT_0 {
				windows.CancelIoEx(h, &ov)
				var n uint32
				windows.GetOverlappedResult(h, &ov, &n, true)
				if err != nil {
					report(err)
				}
				return
			}
			var size uint32
			if err := windows.GetOverlappedResult(h, &ov, &size, false); err != nil {
				report(err)
				return
			}
			// 返回空缓冲表示变更过快已溢出，需上报并重新挂起。
			if size == 0 {
				report(fsnotify.ErrEventOverflow)
				continue
			}
			// 逐条解析 FILE_NOTIFY_INFORMATION：偏移、动作、名称长度各占 4 字节。
			for offset := uint32(0); offset+12 <= size; {
				next := binary.LittleEndian.Uint32(buf[offset:])
				action := binary.LittleEndian.Uint32(buf[offset+4:])
				length := binary.LittleEndian.Uint32(buf[offset+8:])
				// 名称按 UTF-16 编码，长度必须为偶数且不越界。
				if length%2 != 0 || length > size-offset-12 {
					report(fmt.Errorf("invalid directory notification"))
					break
				}
				units := make([]uint16, length/2)
				for i := range units {
					units[i] = binary.LittleEndian.Uint16(buf[offset+12+uint32(i)*2:])
				}
				name := windows.UTF16ToString(units)
				op := fsnotify.Write
				switch action {
				case windows.FILE_ACTION_ADDED, windows.FILE_ACTION_RENAMED_NEW_NAME:
					op = fsnotify.Create
				case windows.FILE_ACTION_REMOVED:
					op = fsnotify.Remove
				case windows.FILE_ACTION_RENAMED_OLD_NAME:
					op = fsnotify.Rename
				}
				select {
				case events <- fsnotify.Event{Name: filepath.Join(parent, name), Op: op}:
				case <-stopping:
					return
				}
				// next 为 0 表示最后一条；否则按声明偏移前进。
				if next == 0 {
					break
				}
				if next < 12 || next > size-offset {
					report(fmt.Errorf("invalid directory notification offset"))
					break
				}
				offset += next
			}
		}
	}()
	if err := <-ready; err != nil {
		closeFn()
		return nil, nil, nil, err
	}
	return events, errorsOut, closeFn, nil
}
