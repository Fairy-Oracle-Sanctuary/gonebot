package logx

import (
	"syscall"
	"unsafe"
)

// detectWindowsLocale 通过 Win32 API 检测系统 UI 语言.
func detectWindowsLocale() Locale {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getUserDefaultUILanguage := kernel32.NewProc("GetUserDefaultUILanguage")

	langID, _, _ := getUserDefaultUILanguage.Call()
	if langID == 0 {
		return EN
	}

	// LANGID: 0x0804 = zh-CN, 0x0404 = zh-TW, 0x0c04 = zh-HK, 0x1004 = zh-SG
	switch uint16(langID) {
	case 0x0804, 0x0404, 0x0c04, 0x1004:
		return ZH
	}
	return EN
}

var _ = unsafe.Sizeof(0)
