package main

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcp"
)

func NewAppToolCallProcessor(stream bool) *appToolCallProcessor {
	return &appToolCallProcessor{stream: stream}
}

type appToolCallProcessor struct {
	stream      bool
	streamIndex int
}

func (tcp *appToolCallProcessor) ShowProgress(tc mcp.ToolCall) {
	if tc.Progress != nil {
		FgYellow.Printf("Progress: %v\n", tc.Progress)
	}
}

func (tcp *appToolCallProcessor) ShowPartialResults(tc mcp.ToolCall) {
	if tc.Result != nil {
		if !tcp.stream {
			FgHiBlue.Printf("PartialResult: %v\n", tc.Result)
			return
		}
		type streamToolCallResult struct {
			Text []string `json:"text"`
		}
		result := aids.MustUnmarshal[streamToolCallResult](tc.Result)
		for ; tcp.streamIndex < len(result.Text); tcp.streamIndex++ { // Something new in result, stream it
			tcp.StreamText(result.Text[tcp.streamIndex], 100)
		}
	}
}

// StreamTextSimple streams text character by character at a constant rate
func (tcp *appToolCallProcessor) StreamText(text string, charsPerSecond int) {
	delay := time.Second / time.Duration(charsPerSecond)
	for _, char := range text {
		FgHiCyan.Print(string(char))
		time.Sleep(delay)
	}
}

func (tcp *appToolCallProcessor) Sample(tc mcp.ToolCall) any {
	FgHiGreen.Printf("SamplingRequest: %v\n", tc.SamplingRequest)
	return nil
}

var actions = map[string]string{"a": "accept", "d": "decline", "c": "cancel"}

// Elicit handles this part of the MCP spec: https://modelcontextprotocol.io/specification/2025-06-18/client/elicitation
func (tcp *appToolCallProcessor) Elicit(tc mcp.ToolCall) any {
	// fmt.Printf("ElicitationRequest: %v\n", tc.ElicitationRequest)
	prompt, answer := FgYellow, FgHiWhite
	prompt.Printf("Elicitation request: %s\n", tc.ElicitationRequest.Message)
	prompt.Print("Do you [A]ccept, [D]ecline, or [C]ancel? ")
	var action string // Action: accept, decline, cancel
	answer.Print()
	fmt.Scan(&action)
	action = strings.ToLower(strings.TrimSpace(action))[0:1]
	result := mcp.ElicitationResult{Action: actions[action], Content: &map[string]any{}}
	if action != "a" {
		return result // No content if decline or cancel
	}

	for propName, schema := range tc.ElicitationRequest.RequestedSchema.Properties {
		required := slices.Contains(tc.ElicitationRequest.RequestedSchema.Required, propName)
		switch s := schema.(type) {
		case mcp.BooleanSchema:
			prompt.Printf("%s-%s (%s%s): ", *s.Title, *s.Description, s.Type, aids.Iif(required, "*", ""))
			answer.Print()
			var boolean bool
			fmt.Scan(&boolean)
			(*result.Content)[propName] = boolean

		case mcp.EnumSchema:
			prompt.Printf("%s-%s (%s%s): ", *s.Title, *s.Description, s.Type, aids.Iif(required, "*", ""))
			for i, v := range s.EnumNames {
				prompt.Printf("    %d: %s\n", i+1, v)
			}
			answer.Print()
			index := 0
			fmt.Scan(&index)
			(*result.Content)[propName] = s.Enum[index-1]

		case mcp.NumberSchema:
			switch s.Type {
			case "integer":
				prompt.Printf("%s-%s (%s%s) [%d-%d]: ", *s.Title, *s.Description, s.Type, aids.Iif(required, "*", ""),
					aids.Iif(s.Minimum == nil, math.MinInt, int(*s.Minimum)),
					aids.Iif(s.Maximum == nil, math.MaxInt, int(*s.Maximum)))
				answer.Print()
				var number int
				fmt.Scan(&number)
				(*result.Content)[propName] = number

			case "number":
				prompt.Printf("%s-%s (%s%s) [%f-%f]: ", *s.Title, *s.Description, s.Type, aids.Iif(required, "*", ""),
					aids.Iif(s.Minimum == nil, math.SmallestNonzeroFloat64, *s.Minimum),
					aids.Iif(s.Maximum == nil, math.MaxFloat64, *s.Maximum))
				answer.Print()
				var number float64
				fmt.Scan(&number)
				(*result.Content)[propName] = number

			default:
				aids.Assert(false, "Unknown number type: "+s.Type)
			}

		case mcp.StringSchema:
			prompt.Printf("%s-%s (%s%s) [%s, %d-%d characters]: ", *s.Title, *s.Description, s.Type, aids.Iif(required, "*", ""),
				*s.Format, aids.Iif(s.MinLength == nil, 0, *s.MinLength),
				aids.Iif(s.MaxLength == nil, 1000, *s.MaxLength))
			line := ""
			fmt.Scanln(&line) //aids.Must(bufio.NewReader(os.Stdin).ReadString('\n'))
			(*result.Content)[propName] = strings.TrimSpace(line)
		}
	}
	return result
}

func (tcp *appToolCallProcessor) Terminated(tc mcp.ToolCall) {
	switch *tc.Status { // The tool call terminated
	case "success":
		if tcp.stream {
			tcp.ShowPartialResults(tc)
		} else {
			FgHiMagenta.Printf("%v\n", tc.Result)
		}
	case "failed":
		FgWhite.And(BgYellow).Printf("%v\n", tc.Error)
		// TODO: Process failure using response.error
	case "canceled":
		FgYellow.And(BgBlue).Printf("Canceled\n")
	}
}
