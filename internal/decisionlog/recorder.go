package decisionlog

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
)

type RecorderStats struct {
	RecordsWritten uint64
	WriteErrors    uint64
}

type Recorder struct {
	mu      sync.Mutex
	w       io.Writer
	closer  io.Closer
	closed  bool
	nextID  atomic.Uint64
	written uint64
	errors  uint64
}

func NewRecorder(w io.Writer) *Recorder {
	return &Recorder{w: w}
}

func OpenRecorder(path string) (*Recorder, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("decisionlog: open %s: %w", path, err)
	}

	return &Recorder{
		w:      file,
		closer: file,
	}, nil
}

func (r *Recorder) NextDecisionID() string {
	if r == nil {
		return ""
	}

	return strconv.FormatUint(r.nextID.Add(1), 10)
}

func (r *Recorder) Record(record Record) error {
	if r == nil || r.w == nil {
		return nil
	}

	data, err := json.Marshal(record)
	if err != nil {
		r.addWriteError()
		return fmt.Errorf("decisionlog: marshal record: %w", err)
	}

	line := make([]byte, 0, len(data)+1)
	line = append(line, data...)
	line = append(line, '\n')

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		r.errors++
		return fmt.Errorf("decisionlog: recorder is closed")
	}

	n, err := r.w.Write(line)
	if err != nil {
		r.errors++
		return fmt.Errorf("decisionlog: write record: %w", err)
	}

	if n != len(line) {
		r.errors++
		return fmt.Errorf("decisionlog: write record: %w", io.ErrShortWrite)
	}

	r.written++

	return nil
}

func (r *Recorder) Stats() RecorderStats {
	if r == nil {
		return RecorderStats{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return RecorderStats{
		RecordsWritten: r.written,
		WriteErrors:    r.errors,
	}
}

func (r *Recorder) Close() error {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true

	if r.closer == nil {
		return nil
	}

	return r.closer.Close()
}

func (r *Recorder) addWriteError() {
	r.mu.Lock()
	r.errors++
	r.mu.Unlock()
}
