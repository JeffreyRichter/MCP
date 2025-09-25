package aids

import (
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"io"
	"path"
	"runtime/debug"
	"strconv"
	"strings"
)

// Iif is "inline if"
func Iif[T any](expression bool, trueVal, falseVal T) T {
	if expression {
		return trueVal
	}
	return falseVal
}

// IsError returns true if err is nil
func IsError(err error) bool { return err != nil }

// Assert panics if condition is false
func Assert(condition bool, v any) {
	if condition {
		return
	}
	if err, ok := v.(error); ok {
		panic(err)
	}
	panic(fmt.Errorf("%#v", v))
}

// Must0 panics if err != nil
func Must0(err error) {
	Assert(!IsError(err), err)
}

// Must returns val if err is nil, otherwise panics with err
func Must[T any](val T, err error) T {
	Assert(!IsError(err), err)
	return val
}

func MustMarshal(v any) jsontext.Value { return Must(json.Marshal(v)) }

func MustUnmarshal[T any](data []byte) T {
	var t T
	Must0(json.Unmarshal(data, &t))
	return t
}

// WriteStack captures the calling goroutine's stack and writes formatted output to w.
func WriteStack(w io.Writer, fi Stack) {
	format := fmt.Sprintf("%%-%ds   %%-%ds   %%s:%%d\n", fi.LongestPackage, fi.LongestFunction)
	for _, f := range fi.Frames {
		fmt.Fprintf(w, format, f.Package, f.Function, path.Join(f.FilePath, f.FileName), f.Line)
	}
}

type Stack struct {
	LongestPackage   int
	LongestFilePath  int
	LongestFileName  int
	LongestFunction  int
	LongestArguments int
	Frames           []*Frame
}
type Frame struct {
	Package   string
	FilePath  string
	FileName  string
	Function  string
	Arguments string
	Line      int64
	Offset    int64
}

// ParseStack parses a stack trace into a slice of frames
func ParseStack(framesToSkip int) Stack {
	stack := string(debug.Stack())
	// fmt.Println(stack)	// For debugging
	fi := Stack{Frames: []*Frame{}}
	framesSkipped := 0
	lines := strings.Split(stack, "\n")
	for l := 0; l < len(lines); l++ {
		line := strings.TrimSpace(lines[l])
		switch {
		case line == "", strings.HasPrefix(line, "goroutine"), strings.HasPrefix(line, "Recovered from panic:"):
			continue
		case strings.HasPrefix(line, "panic"), strings.HasPrefix(line, "runtime/"):
			l++ // Skip the next line
			continue

		default:
			if framesSkipped < framesToSkip {
				framesSkipped++
				l++
				break
			}
			f := &Frame{}
			// Parse the line into package & function name
			parts := strings.SplitN(line, ".", 2)
			f.Package = parts[0]
			paren := strings.Index(strings.TrimSpace(parts[1]), "(") // Find the opening parenthesis for arguments
			f.Function, f.Arguments = parts[1][:paren], parts[1][paren:]
			l++
			if f.Function == "panicStack" {
				continue
			}
			line = lines[l] // Move to the next line which contains file path info
			// Parse the line into filepath, line number, and Offset; Example: C:/Users/jeffreyr/OneDrive/Documents/Projects/GoPlay/src/MCP/zstream/main.go:25 +0xb3
			i := len(line) - 1
			for ; i >= 0; i-- {
				if line[i] == '+' {
					f.Offset, _ = strconv.ParseInt(line[i:], 0, 0) // Base 16 inferred from 0x prefix
					break
				}
			}
			end := i - 1
			for ; i >= 0; i-- {
				if line[i] == ':' {
					f.Line, _ = strconv.ParseInt(line[i+1:end], 10, 0)
					break
				}
			}
			dir, file := path.Split(line[:i])
			f.FilePath, f.FileName = strings.TrimSpace(dir), strings.TrimSpace(file)
			fi.Frames = append(fi.Frames, f)

			fi.LongestPackage = max(fi.LongestPackage, len(f.Package))
			fi.LongestFunction = max(fi.LongestFunction, len(f.Function))
			fi.LongestArguments = max(fi.LongestArguments, len(f.Arguments))
			fi.LongestFilePath = max(fi.LongestFilePath, len(f.FilePath))
			fi.LongestFileName = max(fi.LongestFileName, len(f.FileName))
		}
	}
	return fi
}
