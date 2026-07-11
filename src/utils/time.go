package utils

import (
	"fmt"
	"strconv"
	"sync"
	"time"
)

// Timed returns a function that, when called, prints the time elapsed since Timed
// was called alongside infoStr. The duration is rendered in whichever unit keeps
// it readable (s / ms / µs / ns), so sub-second calls don't collapse to "0.0s".
//
// time.Now() can only resolve down to the OS timer period (often ~0.5–15ms on
// Windows), so anything faster reads as exactly zero. Rather than print a
// misleading "0ns", those are shown as "<{resolution}" — the measured floor below
// which the timer can't see.
func Timed(infoStr string) func() {
	now := time.Now()

	return func() {
		d := time.Since(now)
		s := FormatDuration(d)
		if d == 0 {
			s = "<" + unit(clockResolution())
		}
		fmt.Printf("\t\t[%s] %s\n", s, infoStr)
	}
}

// FormatDuration renders d with the largest unit that keeps the value >= 1
// (s / ms / µs / ns). For sub-millisecond durations it also appends the value in
// milliseconds, so the magnitude is easy to compare: "45.6µs (0.0456ms)".
func FormatDuration(d time.Duration) string {
	s := unit(d)
	if d > 0 && d < time.Millisecond {
		s += " (" + milliStr(d) + "ms)"
	}

	return s
}

// unit renders d as a value plus its largest fitting unit, with no conversion.
func unit(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.2fs", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
	case d >= time.Microsecond:
		return fmt.Sprintf("%.1fµs", float64(d)/float64(time.Microsecond))
	default:
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
}

// milliStr renders d in milliseconds as the shortest exact decimal with no
// exponent, e.g. "0.0456" or "0.000001".
func milliStr(d time.Duration) string {
	return strconv.FormatFloat(float64(d)/float64(time.Millisecond), 'f', -1, 64)
}

// clockResolution is the smallest non-zero interval time.Now() can measure on this
// host, found once by sampling. It's the floor below which a measured duration
// reads as zero.
var clockResolution = sync.OnceValue(func() time.Duration {
	min := time.Hour
	for i := 0; i < 300_000; i++ {
		a := time.Now()
		if d := time.Since(a); d > 0 && d < min {
			min = d
		}
	}
	if min == time.Hour {
		return 0
	}

	return min
})
