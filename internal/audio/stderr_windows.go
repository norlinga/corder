//go:build windows

package audio

func captureNativeStderr(fn func()) string {
	fn()
	return ""
}
