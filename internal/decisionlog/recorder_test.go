package decisionlog

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRecorderWritesJSONL(t *testing.T) {
	var buffer bytes.Buffer
	recorder := NewRecorder(&buffer)

	records := []Record{
		sampleRecord("1"),
		sampleRecord("2"),
	}

	for _, record := range records {
		if err := recorder.Record(record); err != nil {
			t.Fatalf("record %s: %v", record.DecisionID, err)
		}
	}

	output := buffer.String()
	if !strings.HasSuffix(output, "\n") {
		t.Fatal("expected output to end with a newline")
	}

	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != len(records) {
		t.Fatalf("expected %d lines, got %d", len(records), len(lines))
	}

	for i, line := range lines {
		var got Record
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d is not valid JSON: %v", i, err)
		}
		if got.DecisionID != records[i].DecisionID {
			t.Fatalf("expected decision id %s, got %s", records[i].DecisionID, got.DecisionID)
		}
	}

	stats := recorder.Stats()
	if stats.RecordsWritten != 2 {
		t.Fatalf("expected 2 records written, got %d", stats.RecordsWritten)
	}
	if stats.WriteErrors != 0 {
		t.Fatalf("expected 0 write errors, got %d", stats.WriteErrors)
	}
}

func TestRecorderDecisionIDsUniqueConcurrent(t *testing.T) {
	recorder := NewRecorder(io.Discard)
	const count = 100

	ids := make(chan string, count)
	var wg sync.WaitGroup

	for i := 0; i < count; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			ids <- recorder.NextDecisionID()
		}()
	}

	wg.Wait()
	close(ids)

	seen := make(map[string]struct{}, count)
	for id := range ids {
		if id == "" {
			t.Fatal("expected non-empty decision id")
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate decision id %s", id)
		}
		seen[id] = struct{}{}
	}

	if len(seen) != count {
		t.Fatalf("expected %d unique decision ids, got %d", count, len(seen))
	}
}

func TestRecorderConcurrentWrites(t *testing.T) {
	var buffer bytes.Buffer
	recorder := NewRecorder(&buffer)

	const (
		workers          = 8
		recordsPerWorker = 25
	)

	var wg sync.WaitGroup

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)

		go func(worker int) {
			defer wg.Done()

			for i := 0; i < recordsPerWorker; i++ {
				record := sampleRecord(recorder.NextDecisionID())
				record.Run = worker

				if err := recorder.Record(record); err != nil {
					t.Errorf("worker %d record %d: %v", worker, i, err)
					return
				}
			}
		}(worker)
	}

	wg.Wait()

	output := strings.TrimSuffix(buffer.String(), "\n")
	lines := strings.Split(output, "\n")
	if len(lines) != workers*recordsPerWorker {
		t.Fatalf("expected %d lines, got %d", workers*recordsPerWorker, len(lines))
	}

	for i, line := range lines {
		var got Record
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d is not valid JSON: %v", i, err)
		}
	}

	stats := recorder.Stats()
	if stats.RecordsWritten != workers*recordsPerWorker {
		t.Fatalf("expected %d records written, got %d", workers*recordsPerWorker, stats.RecordsWritten)
	}
	if stats.WriteErrors != 0 {
		t.Fatalf("expected 0 write errors, got %d", stats.WriteErrors)
	}
}

func TestDisabledRecorder(t *testing.T) {
	var nilRecorder *Recorder

	if err := nilRecorder.Record(sampleRecord("1")); err != nil {
		t.Fatalf("nil recorder record: %v", err)
	}
	if err := nilRecorder.Close(); err != nil {
		t.Fatalf("nil recorder close: %v", err)
	}
	if id := nilRecorder.NextDecisionID(); id != "" {
		t.Fatalf("expected empty decision id from nil recorder, got %s", id)
	}
	if stats := nilRecorder.Stats(); stats != (RecorderStats{}) {
		t.Fatalf("expected zero stats from nil recorder, got %+v", stats)
	}

	disabled := NewRecorder(nil)
	if err := disabled.Record(sampleRecord("1")); err != nil {
		t.Fatalf("disabled recorder record: %v", err)
	}
	if stats := disabled.Stats(); stats != (RecorderStats{}) {
		t.Fatalf("expected zero stats from disabled recorder, got %+v", stats)
	}
}

func TestRecorderWriteError(t *testing.T) {
	writerErr := errors.New("write failed")
	recorder := NewRecorder(&failingWriter{err: writerErr})

	if err := recorder.Record(sampleRecord("1")); err == nil {
		t.Fatal("expected write error")
	}

	stats := recorder.Stats()
	if stats.WriteErrors != 1 {
		t.Fatalf("expected 1 write error, got %d", stats.WriteErrors)
	}
	if stats.RecordsWritten != 0 {
		t.Fatalf("expected 0 records written, got %d", stats.RecordsWritten)
	}

	if err := recorder.Record(sampleRecord("2")); err == nil {
		t.Fatal("expected second write error")
	}

	stats = recorder.Stats()
	if stats.WriteErrors != 2 {
		t.Fatalf("expected 2 write errors, got %d", stats.WriteErrors)
	}
	if stats.RecordsWritten != 0 {
		t.Fatalf("expected 0 records written, got %d", stats.RecordsWritten)
	}
}

func TestOpenRecorderAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")

	for _, id := range []string{"first", "second"} {
		recorder, err := OpenRecorder(path)
		if err != nil {
			t.Fatalf("open recorder: %v", err)
		}

		record := sampleRecord(id)
		if err := recorder.Record(record); err != nil {
			_ = recorder.Close()
			t.Fatalf("record %s: %v", id, err)
		}
		if err := recorder.Close(); err != nil {
			t.Fatalf("close recorder: %v", err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read decisions: %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 appended lines, got %d", len(lines))
	}

	for i, expected := range []string{"first", "second"} {
		var got Record
		if err := json.Unmarshal([]byte(lines[i]), &got); err != nil {
			t.Fatalf("line %d is not valid JSON: %v", i, err)
		}
		if got.DecisionID != expected {
			t.Fatalf("expected decision id %s, got %s", expected, got.DecisionID)
		}
	}
}

type failingWriter struct {
	err error
}

func (w *failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func sampleRecord(id string) Record {
	return Record{
		SchemaVersion:   SchemaVersion,
		DecisionID:      id,
		RunID:           "run-1",
		SelectionTime:   time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC),
		Scenario:        "baseline",
		Strategy:        "adaptive",
		Run:             1,
		SelectedBackend: "backend-1",
		PolicyVersion:   "policy-v1",
		Candidates:      []CandidateSnapshot{},
		Outcome:         Outcome{},
	}
}
