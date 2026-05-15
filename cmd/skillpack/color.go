package main

import (
	"os"
	"strings"
)

// ANSI escape codes.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
)

// colorsEnabled is true when stdout is an interactive terminal and NO_COLOR is not set.
var colorsEnabled = func() bool {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	if fi, err := os.Stdout.Stat(); err == nil {
		return (fi.Mode() & os.ModeCharDevice) != 0
	}
	return false
}()

func bold(s string) string   { return ansiWrap(ansiBold, s) }
func green(s string) string  { return ansiWrap(ansiGreen, s) }
func yellow(s string) string { return ansiWrap(ansiYellow, s) }
func red(s string) string    { return ansiWrap(ansiRed, s) }
func cyan(s string) string   { return ansiWrap(ansiCyan, s) }

func ansiWrap(code, s string) string {
	if !colorsEnabled || s == "" {
		return s
	}
	return code + s + ansiReset
}
