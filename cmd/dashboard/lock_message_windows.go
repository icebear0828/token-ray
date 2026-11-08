//go:build windows

package main

import "golang.org/x/sys/windows"

func showAlreadyRunningMessage(dataDir string) {
	message := "AI Flight Dashboard is already running.\n\n" +
		"If you cannot see its window, click the tray icon in the notification area, " +
		"or end the existing process in Task Manager.\n\nData directory:\n" + dataDir
	windows.MessageBox(
		0,
		windows.StringToUTF16Ptr(message),
		windows.StringToUTF16Ptr("AI Flight Dashboard"),
		windows.MB_OK|windows.MB_ICONINFORMATION|windows.MB_SETFOREGROUND,
	)
}
