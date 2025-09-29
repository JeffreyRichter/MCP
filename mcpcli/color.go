package main

import (
	"fmt"
)

type Color uint32

const (
	FgBlack   Color = 0x001e
	FgRed     Color = 0x001f
	FgGreen   Color = 0x0020
	FgYellow  Color = 0x0021
	FgBlue    Color = 0x0022
	FgMagenta Color = 0x0023
	FgCyan    Color = 0x0024
	FgWhite   Color = 0x0025
)

const (
	FgHiBlack   Color = 0x005a
	FgHiRed     Color = 0x005b
	FgHiGreen   Color = 0x005c
	FgHiYellow  Color = 0x005d
	FgHiBlue    Color = 0x005e
	FgHiMagenta Color = 0x005f
	FgHiCyan    Color = 0x0060
	FgHiWhite   Color = 0x0061
)

const (
	BgBlack   Color = 0x2800
	BgRed     Color = 0x2900
	BgGreen   Color = 0x2A00
	BgYellow  Color = 0x2B00
	BgBlue    Color = 0x2C00
	BgMagenta Color = 0x2D00
	BgCyan    Color = 0x2E00
	BgWhite   Color = 0x2F00
)

const (
	BgHiBlack   Color = 0x6400
	BgHiRed     Color = 0x6500
	BgHiGreen   Color = 0x6600
	BgHiYellow  Color = 0x6700
	BgHiBlue    Color = 0x6800
	BgHiMagenta Color = 0x6900
	BgHiCyan    Color = 0x6A00
	BgHiWhite   Color = 0x6B00
)

const Keep Color = 0x00010000

const (
	fgMask    Color = 0xFFFFFF00
	bgMask    Color = 0xFFFF00FF
	keepMask  Color = 0xFF00FFFF
	bgShift   int   = 8
	keepShift int   = 16
)

func (c Color) Print(a ...any)                 { c.set(); fmt.Print(a...); c.reset() }
func (c Color) Printf(format string, a ...any) { c.set(); fmt.Printf(format, a...); c.reset() }
func (c Color) Println(a ...any)               { c.set(); fmt.Println(a...); c.reset() }
func (c Color) set() {
	fg, bg := (c &^ fgMask), (c&^bgMask)>>bgShift
	switch {
	case fg == 0 && bg == 0:
		return // No color to set
	case fg != 0 && bg == 0:
		fmt.Printf("%s[%dm", escape, fg)
	case fg == 0 && bg != 0:
		fmt.Printf("%s[%dm", escape, bg)
	default: // Both fg and bg set
		fmt.Printf("%s[%d;%dm", escape, fg, bg)
	}
}
func (c Color) reset() {
	//	if c&^keepMask == 0 {
	fmt.Printf("%s[0m", escape)
	//	}
}
func (c Color) Keep(keep bool) Color {
	bit := 0
	if keep {
		bit = 1
	}
	return (c & keepMask) | Color((bit << keepShift))
}
func (c Color) And(and Color) Color {
	switch {
	case and.isFg():
		return (c & fgMask) | and
	case and.isBg():
		return (c & bgMask) | and
	default:
		return (c & keepMask) | and
	}
}
func (c Color) isFg() bool {
	return (c >= FgBlack && c <= FgWhite) || (c >= FgHiBlack && c <= FgHiWhite)
}
func (c Color) isBg() bool {
	return (c >= BgBlack && c <= BgWhite) || (c >= BgHiBlack && c <= BgHiWhite)
}

const escape = "\x1b"

// Insirpation: https://github.com/fatih/color/blob/main/color.go
