package main

import "fmt"

func main() {
	FgHiBlack.And(BgWhite).Print("MCP Client started\n")

	FgBlack.Print("MCP Client started\n")
	FgYellow.And(BgBlue).Printf("MCP Client started\n")
	fmt.Printf("MCP Client started\n")

	FgCyan.Printf("MCP Client started\n")
	FgGreen.Printf("MCP Client started\n")
	FgRed.Printf("MCP Client started\n")
	FgMagenta.Printf("MCP Client started\n")
	FgBlue.Printf("MCP Client started\n")
	FgWhite.Printf("MCP Client started\n")
}
