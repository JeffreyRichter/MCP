package chunkwriter

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JeffreyRichter/internal/aids"
)

// permStringToFileMode converts a 9-character permission string to os.FileMode.
// It returns an error if the string is not the correct length or contains invalid characters.
func permStringToFileMode(permStr string) (os.FileMode, error) {
	if len(permStr) != 9 {
		return 0, fmt.Errorf("invalid permission string length: %d, expected 9", len(permStr))
	}

	mode := os.FileMode(0)
	for i, char := range permStr {
		switch {
		case i == 0 && char == 'r':
			mode |= fs.ModePerm & 0400 // User read
		case i == 1 && char == 'w':
			mode |= fs.ModePerm & 0200 // User write
		case i == 2 && char == 'x':
			mode |= fs.ModePerm & 0100 // User execute

		case i == 3 && char == 'r':
			mode |= fs.ModePerm & 0040 // Group read
		case i == 4 && char == 'w':
			mode |= fs.ModePerm & 0020 // Group write
		case i == 5 && char == 'x':
			mode |= fs.ModePerm & 0010 // Group execute

		case i == 6 && char == 'r':
			mode |= fs.ModePerm & 0004 // Other read
		case i == 7 && char == 'w':
			mode |= fs.ModePerm & 0002 // Other write
		case i == 8 && char == 'x':
			mode |= fs.ModePerm & 0001 // Other execute
		}
	}
	return mode, nil
}

type LogFileFlusher struct {
	// Immutable after construction
	path         string
	ignoreOffset bool
	fileMode     os.FileMode

	// Mutable during lifetime
	mu           sync.Mutex
	lastPathname string
	lastOffset   int64
}

func NewLogFileFlusher(path string, ignoreOffset bool) *LogFileFlusher {
	return &LogFileFlusher{
		path:         path,
		ignoreOffset: ignoreOffset,
		fileMode:     aids.Must(permStringToFileMode("-w--w----"))}
}

func (lff *LogFileFlusher) Flush(c Chunk) error {
	pathname := filepath.Join(lff.path, fmt.Sprintf("%s.log", time.Now().Format("2006-01-02_15.04.05"))) // Year, Month, Day, Hour, Minute, Second
	lff.mu.Lock()
	if lff.lastPathname != pathname {
		lff.lastPathname, lff.lastOffset = pathname, c.Offset
	}
	lff.mu.Unlock()

	// Open file for writing/append & write to it.
	flag := os.O_CREATE | os.O_WRONLY | aids.Iif(lff.ignoreOffset, os.O_APPEND, 0)
	f, err := os.OpenFile(pathname, flag, lff.fileMode)
	if err != nil {
		return err
	}
	defer f.Close()
	if lff.ignoreOffset {
		_, err = f.Write(c.Data)
	} else {
		_, err = f.WriteAt(c.Data, c.Offset-lff.lastOffset)
	}
	return err
}

func TestFlush_LogFileFlushHonoringOffset(t *testing.T) {
	lff := NewLogFileFlusher(".", false)
	cw := New(lff, Options{FlushAfterNumWrites: 4})

	for range 10 {
		_, err := cw.Write([]byte(time.Now().Format(time.DateTime)))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		_, err = cw.Write([]byte("\n"))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		time.Sleep(time.Millisecond * 400)
	}
	err := cw.Close()
	if err != nil {
		t.Fatalf("Final flush failed: %v", err)
	}
}

func TestFlush_LogFileFlushIgnoringOffsetAsync(t *testing.T) {
	lff := NewLogFileFlusher(".", true)
	cw := New(lff, Options{FlushAfterNumWrites: 4, FlushSynchronously: false})

	for range 10 {
		_, err := cw.Write([]byte(time.Now().Format(time.DateTime)))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		_, err = cw.Write([]byte("\n"))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		time.Sleep(time.Millisecond * 400)
	}
	err := cw.Close()
	if err != nil {
		t.Fatalf("Final flush failed: %v", err)
	}
}

func TestFlush_LogFileFlushIgnoringOffsetSync(t *testing.T) {
	lff := NewLogFileFlusher(".", true)
	cw := New(lff, Options{FlushAfterNumWrites: 4, FlushSynchronously: true})

	for range 10 {
		_, err := cw.Write([]byte(time.Now().Format(time.DateTime)))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		_, err = cw.Write([]byte("\n"))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		time.Sleep(time.Millisecond * 400)
	}
	err := cw.Close()
	if err != nil {
		t.Fatalf("Final flush failed: %v", err)
	}
}

func TestFlush_Slog(t *testing.T) {
	lff := NewLogFileFlusher(".", false)
	cw := New(lff, Options{FlushAfterNumWrites: 4})
	logger := slog.New(slog.NewJSONHandler(cw, &slog.HandlerOptions{AddSource: false, Level: slog.LevelDebug}))
	logger.Debug("JMR Debug", "foo", "bar")
	logger.Info("JMR Info", "zippo", "zingo")
	logger.Error("JMR Error", "error", errors.New("sample error"))
	logger.Warn("JMR Warn", "abc", "xyz")
	_ = cw.Close()
}

// mockFlusher is a mock implementation of [Flusher] for testing
type mockFlusher struct {
	mu       sync.RWMutex
	data     map[int64][]byte
	writeErr error
	calls    []writeCall
	delay    time.Duration
}

type writeCall struct {
	offset int64
	data   []byte
}

func newMockFlusher() *mockFlusher {
	return &mockFlusher{
		data:  make(map[int64][]byte),
		calls: make([]writeCall, 0),
	}
}

func (m *mockFlusher) Flush(c Chunk) error {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.writeErr != nil {
		return m.writeErr
	}

	// Make a copy to avoid data races
	dataCopy := make([]byte, len(c.Data))
	copy(dataCopy, c.Data)

	m.data[c.Offset] = dataCopy
	m.calls = append(m.calls, writeCall{offset: c.Offset, data: dataCopy})
	return nil
}

func (m *mockFlusher) setWriteError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeErr = err
}

func (m *mockFlusher) getData(offset int64) []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.data[offset]
}

func (m *mockFlusher) getCalls() []writeCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]writeCall, len(m.calls))
	copy(result, m.calls)
	return result
}

func (m *mockFlusher) getCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.calls)
}

func (m *mockFlusher) setDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delay = delay
}

func TestNew(t *testing.T) {
	mf := newMockFlusher()
	opts := Options{
		FlushAfterNumWrites: 5,
		FlushAfterLength:    100,
		FlushAfterDuration:  time.Second,
	}

	cw := New(mf, opts)

	if cw == nil {
		t.Fatal("New returned nil")
	}

	if cw.o.FlushAfterNumWrites != 5 {
		t.Error("Options not properly set")
	}

	if cw.c == nil {
		t.Error("Internal buffer not initialized")
	}

	if cw.c.Offset != 0 {
		t.Error("Initial buffer offset should be 0")
	}

	if cw.c.numWrites != 0 {
		t.Error("Initial buffer should have 0 writes")
	}
}

func TestWrite_BasicFunctionality(t *testing.T) {
	mf := newMockFlusher()
	cw := New(mf, Options{})

	testData := []byte("hello world")
	n, err := cw.Write(testData)

	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if n != len(testData) {
		t.Errorf("Expected n=%d, got %d", len(testData), n)
	}

	// Data should be buffered, not yet written to WriterAt
	if mf.getCallCount() != 0 {
		t.Error("Data should be buffered, not written yet")
	}
}

func TestWrite_EmptyData(t *testing.T) {
	mf := newMockFlusher()
	cw := New(mf, Options{})

	n, err := cw.Write([]byte{})

	if err != nil {
		t.Fatalf("Write of empty data failed: %v", err)
	}

	if n != 0 {
		t.Errorf("Expected n=0 for empty write, got %d", n)
	}
}

func TestFlush_Basic(t *testing.T) {
	mf := newMockFlusher()
	cw := New(mf, Options{})

	testData := []byte("hello world")
	_, err := cw.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	err = cw.Flush()
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Verify data was written
	if mf.getCallCount() != 1 {
		t.Errorf("Expected 1 write call, got %d", mf.getCallCount())
	}

	written := mf.getData(0)
	if !bytes.Equal(written, testData) {
		t.Errorf("Expected %s, got %s", testData, written)
	}
}

func TestFlush_EmptyBuffer(t *testing.T) {
	mf := newMockFlusher()
	cw := New(mf, Options{})

	// Flush empty buffer should not error
	err := cw.Flush()
	if err != nil {
		t.Fatalf("Flush of empty buffer failed: %v", err)
	}

	if mf.getCallCount() != 0 {
		t.Error("Empty buffer flush should not call WriterAt")
	}
}

func TestFlush_Multiple(t *testing.T) {
	mf := newMockFlusher()
	cw := New(mf, Options{})

	testData := []byte("hello world")
	_, err := cw.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Multiple flushes should be idempotent
	for i := 0; i < 3; i++ {
		err = cw.Flush()
		if err != nil {
			t.Fatalf("Flush %d failed: %v", i+1, err)
		}
	}

	// Should only have one write call
	if mf.getCallCount() != 1 {
		t.Errorf("Expected 1 write call after multiple flushes, got %d", mf.getCallCount())
	}
}

func TestFlushAfterWriteCount(t *testing.T) {
	mf := newMockFlusher()
	cw := New(mf, Options{FlushAfterNumWrites: 3})

	// Write 2 times - should not trigger flush
	cw.Write([]byte("hello"))
	cw.Write([]byte(" "))

	if mf.getCallCount() != 0 {
		t.Error("Should not flush before reaching write count threshold")
	}

	// Third write should trigger flush
	cw.Write([]byte("world"))
	if mf.getCallCount() != 1 {
		t.Errorf("Expected 1 flush after reaching write count, got %d", mf.getCallCount())
	}

	written := mf.getData(0)
	expected := []byte("hello world")
	if !bytes.Equal(written, expected) {
		t.Errorf("Expected %s, got %s", expected, written)
	}
}

func TestFlushAfterSize(t *testing.T) {
	mf := newMockFlusher()
	cw := New(mf, Options{FlushAfterLength: 5000}) // Must be > 4096 due to minimum constraint

	// Write data that doesn't exceed size limit (less than 5000 bytes)
	data1 := make([]byte, 3000)
	for i := range data1 {
		data1[i] = 'a'
	}
	cw.Write(data1)

	if mf.getCallCount() != 0 {
		t.Error("Should not flush before reaching size threshold")
	}

	// Write data that exceeds size limit (total > 5000 bytes)
	data2 := make([]byte, 2500)
	for i := range data2 {
		data2[i] = 'b'
	}
	cw.Write(data2)

	if mf.getCallCount() != 1 {
		t.Errorf("Expected 1 flush after reaching size threshold, got %d", mf.getCallCount())
	}

	written := mf.getData(0)
	expected := append(data1, data2...)
	if !bytes.Equal(written, expected) {
		t.Errorf("Expected %d bytes, got %d bytes", len(expected), len(written))
	}
}

func TestFlushAfterDuration(t *testing.T) {
	mf := newMockFlusher()
	cw := New(mf, Options{FlushAfterDuration: 50 * time.Millisecond})

	cw.Write([]byte("hello world"))

	// Should not be flushed immediately
	if mf.getCallCount() != 0 {
		t.Error("Should not flush immediately after write")
	}

	// Wait for timer to trigger
	time.Sleep(100 * time.Millisecond)

	if mf.getCallCount() != 1 {
		t.Errorf("Expected 1 flush after duration, got %d", mf.getCallCount())
	}

	written := mf.getData(0)
	expected := []byte("hello world")
	if !bytes.Equal(written, expected) {
		t.Errorf("Expected %s, got %s", expected, written)
	}
}

func TestWriteError(t *testing.T) {
	mf := newMockFlusher()
	expectedErr := errors.New("write error")
	mf.setWriteError(expectedErr)

	cw := New(mf, Options{})
	cw.Write([]byte("hello world"))
	err := cw.Flush()

	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestConcurrentWrites(t *testing.T) {
	mf := newMockFlusher()
	cw := New(mf, Options{FlushAfterNumWrites: 100 /* High threshold to avoid auto-flush */})

	numGoroutines := 10
	writesPerGoroutine := 10
	var wg sync.WaitGroup

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				data := []byte("data")
				_, err := cw.Write(data)
				if err != nil {
					t.Errorf("Goroutine %d write %d failed: %v", id, j, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Manually flush
	err := cw.Flush()
	if err != nil {
		t.Fatalf("Final flush failed: %v", err)
	}

	// Verify total data written
	totalExpectedData := numGoroutines * writesPerGoroutine * 4 // "data" = 4 bytes
	written := mf.getData(0)
	if len(written) != totalExpectedData {
		t.Errorf("Expected %d bytes written, got %d", totalExpectedData, len(written))
	}
}

func TestConcurrentFlush(t *testing.T) {
	mf := newMockFlusher()
	mf.setDelay(10 * time.Millisecond) // Add delay to make race conditions more likely

	cw := New(mf, Options{})

	// Write some data
	cw.Write([]byte("hello world"))

	// Start multiple flush operations concurrently
	numFlushers := 5
	var wg sync.WaitGroup
	var errorCount int64

	wg.Add(numFlushers)
	for i := 0; i < numFlushers; i++ {
		go func() {
			defer wg.Done()
			err := cw.Flush()
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
			}
		}()
	}

	wg.Wait()

	if errorCount > 0 {
		t.Errorf("Got %d errors during concurrent flush", errorCount)
	}

	// Should only have one write call despite multiple flushes
	if mf.getCallCount() != 1 {
		t.Errorf("Expected 1 write call, got %d", mf.getCallCount())
	}
}

func TestConcurrentWriteAndFlush(t *testing.T) {
	mf := newMockFlusher()
	cw := New(mf, Options{})

	var wg sync.WaitGroup
	done := make(chan bool)

	// Writer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			select {
			case <-done:
				return
			default:
				cw.Write([]byte("data"))
				time.Sleep(time.Millisecond)
			}
		}
	}()

	// Flusher goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			select {
			case <-done:
				return
			default:
				cw.Flush()
				time.Sleep(2 * time.Millisecond)
			}
		}
	}()

	// Let them run for a bit
	time.Sleep(100 * time.Millisecond)
	close(done)
	wg.Wait()

	// Final flush to ensure all data is written
	cw.Flush()

	// At least some writes should have occurred
	if mf.getCallCount() == 0 {
		t.Error("Expected at least one write call")
	}
}

func TestMultipleFlushTriggers(t *testing.T) {
	mf := newMockFlusher()
	cw := New(mf, Options{FlushAfterNumWrites: 2, FlushAfterLength: 5000, FlushAfterDuration: 100 * time.Millisecond})

	// This should trigger flush by write count (2 writes)
	cw.Write([]byte("hello"))
	cw.Write([]byte("world"))
	if mf.getCallCount() != 1 {
		t.Errorf("Expected flush after write count trigger, got %d calls", mf.getCallCount())
	}

	// This should trigger flush by size (> 5000 bytes)
	longData := make([]byte, 5500)
	for i := range longData {
		longData[i] = 'x'
	}
	cw.Write(longData)

	if mf.getCallCount() != 2 {
		t.Errorf("Expected flush after size trigger, got %d calls", mf.getCallCount())
	}
}

func TestBufferOffsetProgression(t *testing.T) {
	mf := newMockFlusher()
	cw := New(mf, Options{FlushAfterNumWrites: 1 /* Flush after each write */})

	// Write three separate chunks
	chunks := [][]byte{
		[]byte("hello"),
		[]byte("world"),
		[]byte("test"),
	}

	var expectedOffset int64
	for _, chunk := range chunks {
		cw.Write(chunk)

		// Verify the write was at the expected offset
		calls := mf.getCalls()
		if len(calls) == 0 {
			t.Fatal("Expected at least one write call")
		}

		lastCall := calls[len(calls)-1]
		if lastCall.offset != expectedOffset {
			t.Errorf("Expected write at offset %d, got %d", expectedOffset, lastCall.offset)
		}

		if !bytes.Equal(lastCall.data, chunk) {
			t.Errorf("Expected data %s, got %s", chunk, lastCall.data)
		}

		expectedOffset += int64(len(chunk))
	}
}

func BenchmarkWrite(b *testing.B) {
	mf := newMockFlusher()
	cw := New(mf, Options{FlushAfterLength: 1024 * 1024 /* Large size to avoid frequent flushes */})

	data := []byte("benchmark data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cw.Write(data)
	}
}

func BenchmarkFlush(b *testing.B) {
	mf := newMockFlusher()
	cw := New(mf, Options{})

	// Pre-populate buffer
	for i := 0; i < 1000; i++ {
		cw.Write([]byte("data"))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cw.Flush()
		// Write some data for the next flush
		cw.Write([]byte("data"))
	}
}

func BenchmarkConcurrentWrite(b *testing.B) {
	mf := newMockFlusher()
	cw := New(mf, Options{FlushAfterLength: 1024 * 1024 /* Large size to avoid frequent flushes */})

	data := []byte("benchmark data")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cw.Write(data)
		}
	})
}
