package service

import "unicode/utf8"

func truncateString(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

func containsInt64(items []int64, needle int64) bool {
	for _, v := range items {
		if v == needle {
			return true
		}
	}
	return false
}
