package main

import (
	"fmt"
	"time"
)

// StreamTextSimple streams text character by character at a constant rate
func StreamTextSimple(text string, charsPerSecond int) {
	delay := time.Second / time.Duration(charsPerSecond)
	for _, char := range text {
		fmt.Print(string(char))
		time.Sleep(delay)
	}
	fmt.Println()
}

func main() {
	text, done := []string{}, false
	go func() {
		for t := range 10 {
			time.Sleep(60 * time.Millisecond)
			text = append(text, fmt.Sprintf("%d: Hello! I'm an AI assistant. I can help you with a variety of tasks including answering questions, writing content, analyzing data, and much more. What would you like to work on today?", t))
		}
		done = true
	}()

	for t := 0; true; {
		if len(text) == t && !done {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		StreamTextSimple(text[t], 70)
		t++
		if t == len(text) && done {
			break
		}
	}
}
