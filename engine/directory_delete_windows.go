// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

func removeEmptyDirectory(root *os.Root, name string) error {
	if err := validatePath(name); err != nil {
		return err
	}
	base, err := root.Open(".")
	if err != nil {
		return err
	}
	defer base.Close()
	native, err := windows.NewNTUnicodeString(filepath.FromSlash(name))
	if err != nil {
		return err
	}
	attrs := windows.OBJECT_ATTRIBUTES{
		RootDirectory: windows.Handle(base.Fd()), ObjectName: native,
		Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	attrs.Length = uint32(unsafe.Sizeof(attrs))
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, windows.DELETE|windows.SYNCHRONIZE, &attrs, &status, nil, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN,
		windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT,
		0, 0)
	if err == nil {
		defer windows.CloseHandle(handle)
		// POSIX disposition unlinks immediately even with an active directory
		// watcher. Older filesystems can use the ordinary disposition fallback.
		flags := uint32(windows.FILE_DISPOSITION_DELETE | windows.FILE_DISPOSITION_POSIX_SEMANTICS)
		err = windows.NtSetInformationFile(handle, &status, (*byte)(unsafe.Pointer(&flags)), 4, windows.FileDispositionInformationEx)
		if err != nil {
			remove := byte(1)
			err = windows.NtSetInformationFile(handle, &status, &remove, 1, windows.FileDispositionInformation)
		}
	}
	if s, ok := err.(windows.NTStatus); ok {
		err = s.Errno()
	}
	if err == windows.ERROR_DIR_NOT_EMPTY {
		return errDirectoryNotEmpty
	}
	return err
}
