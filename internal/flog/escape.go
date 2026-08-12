package flog

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func isCtrl(r rune) bool {
	return r < 0x20 || (r >= 0x7f && r <= 0x9f)
}

func escape(s string) string {
	if strings.IndexFunc(s, isCtrl) < 0 && utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case isCtrl(r):
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}
