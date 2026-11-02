package main

import (
	"syscall"
)

func hideConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")

	hwnd, _, _ := getConsoleWindow.Call()

	if hwnd != 0 {
		user32 := syscall.NewLazyDLL("user32.dll")
		showWindow := user32.NewProc("ShowWindow")

		showWindow.Call(hwnd, 0)
	}
}
