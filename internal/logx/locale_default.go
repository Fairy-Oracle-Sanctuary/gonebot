//go:build !windows

package logx

// detectWindowsLocale 非 Windows 平台返回 EN (由环境变量 LANG/LC_ALL 覆盖).
func detectWindowsLocale() Locale {
	return EN
}
