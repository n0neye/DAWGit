package watch

import (
	"context"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// watch sends changed paths ("" when the buffer overflowed) until ctx ends.
func watch(ctx context.Context, root string, events chan<- string) error {
	path, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(path, windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	// The read below blocks; cancelling it is how the watcher stops.
	stop := context.AfterFunc(ctx, func() { windows.CancelIoEx(h, nil) })
	go func() {
		defer close(events)
		defer windows.CloseHandle(h)
		defer stop()
		const mask = windows.FILE_NOTIFY_CHANGE_FILE_NAME | windows.FILE_NOTIFY_CHANGE_DIR_NAME |
			windows.FILE_NOTIFY_CHANGE_SIZE | windows.FILE_NOTIFY_CHANGE_LAST_WRITE
		buf := make([]byte, 64*1024)
		for ctx.Err() == nil {
			var n uint32
			if err := windows.ReadDirectoryChanges(h, &buf[0], uint32(len(buf)), true, mask, &n, nil, 0); err != nil {
				return
			}
			if n == 0 { // overflow: the changes did not fit
				send(ctx, events, "")
				continue
			}
			for off := uint32(0); ; {
				info := (*windows.FileNotifyInformation)(unsafe.Pointer(&buf[off]))
				name := windows.UTF16ToString(unsafe.Slice(&info.FileName, info.FileNameLength/2))
				send(ctx, events, filepath.ToSlash(name))
				if info.NextEntryOffset == 0 {
					break
				}
				off += info.NextEntryOffset
			}
		}
	}()
	return nil
}

func send(ctx context.Context, events chan<- string, p string) {
	select {
	case events <- p:
	case <-ctx.Done():
	}
}
