package app

func msgRunes(key string) []rune {
	if len(key) == 1 {
		return []rune(key)
	}
	return nil
}

func truncate(s string, width int) string {
	if len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}

func clampBitrate(n int) int {
	values := []int{96, 128, 160, 192, 256, 320}
	best := values[0]
	for _, v := range values {
		if n <= v {
			return v
		}
		best = v
	}
	return best
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
