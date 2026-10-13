package main

import "syscall"

// The browser always hands its native messaging host a pipe on stdin, while a
// double-click or a console launch leaves a console (or no usable stdin).
func isHostMode() bool {
	h, err := syscall.GetStdHandle(syscall.STD_INPUT_HANDLE)
	if err != nil {
		return false
	}
	t, err := syscall.GetFileType(h)
	if err != nil {
		return false
	}
	return t == syscall.FILE_TYPE_PIPE
}
