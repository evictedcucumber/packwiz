//go:build windows

package ui

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

// isTerminal reports whether f is a terminal that shows escape sequences: a console that can be made to, or the
// terminal of Cygwin, MSYS2 or Git Bash, which isn't a console but a pipe.
func isTerminal(f *os.File) bool {
	if term.IsTerminal(int(f.Fd())) {
		return enableVirtualTerminal(f)
	}
	return isCygwinTerminal(f)
}

// prepare gets f ready to show escape sequences, if it is a console, which has them off until a program asks for them.
func prepare(f *os.File) {
	enableVirtualTerminal(f)
}

// enableVirtualTerminal turns on the console's handling of escape sequences, which the classic Windows console has off
// until a program asks for it. It reports whether the console handles them.
func enableVirtualTerminal(f *os.File) bool {
	handle := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}

// isCygwinTerminal reports whether f is the terminal of Cygwin, MSYS2 or Git Bash (mintty). Their terminals aren't
// consoles, so a program is given a pipe to what is a pseudo-terminal, which is only known by its name.
func isCygwinTerminal(f *os.File) bool {
	handle := windows.Handle(f.Fd())
	if fileType, err := windows.GetFileType(handle); err != nil || fileType != windows.FILE_TYPE_PIPE {
		return false
	}

	// A FILE_NAME_INFO: the length of the name in bytes (a uint32, which is two of the uint16s), then the name
	var info [2 + windows.MAX_PATH]uint16
	if err := windows.GetFileInformationByHandleEx(handle, windows.FileNameInfo, (*byte)(unsafe.Pointer(&info[0])), uint32(len(info)*2)); err != nil {
		return false
	}
	length := int(*(*uint32)(unsafe.Pointer(&info[0]))) / 2
	if length > len(info)-2 {
		return false
	}
	return isCygwinPipeName(windows.UTF16ToString(info[2 : 2+length]))
}
