package server

import (
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// LogBuffer is a thread-safe ring buffer that captures recent log entries
type LogBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	pos     int
	size    int
	wrapped bool
}

// NewLogBuffer creates a new log buffer with the specified capacity
func NewLogBuffer(capacity int) *LogBuffer {
	return &LogBuffer{
		entries: make([]LogEntry, capacity),
		size:    capacity,
	}
}

// Write implements io.Writer for zerolog integration
func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	// This is called by zerolog with JSON-formatted log lines
	// For simplicity, we won't parse it here - just store as message
	return len(p), nil
}

// WriteLevel implements zerolog.LevelWriter
func (lb *LogBuffer) WriteLevel(level zerolog.Level, p []byte) (n int, err error) {
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level.String(),
		Message:   string(p),
		Fields:    make(map[string]interface{}),
	}

	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.entries[lb.pos] = entry
	lb.pos++
	if lb.pos >= lb.size {
		lb.pos = 0
		lb.wrapped = true
	}

	return len(p), nil
}

// GetRecent returns the N most recent log entries
func (lb *LogBuffer) GetRecent(limit int) []LogEntry {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	if limit <= 0 || limit > lb.size {
		limit = lb.size
	}

	var result []LogEntry

	if !lb.wrapped {
		// Buffer not yet full, return from start to pos
		count := lb.pos
		if count > limit {
			count = limit
		}
		start := lb.pos - count
		if start < 0 {
			start = 0
		}
		result = make([]LogEntry, count)
		copy(result, lb.entries[start:lb.pos])
	} else {
		// Buffer is full and wrapped
		if limit >= lb.size {
			// Return all entries in chronological order
			result = make([]LogEntry, lb.size)
			copy(result, lb.entries[lb.pos:])
			copy(result[lb.size-lb.pos:], lb.entries[:lb.pos])
		} else {
			// Return last N entries
			result = make([]LogEntry, limit)
			start := lb.pos - limit
			if start < 0 {
				start += lb.size
				// Wrap around
				n1 := lb.size - start
				copy(result, lb.entries[start:])
				copy(result[n1:], lb.entries[:lb.pos])
			} else {
				copy(result, lb.entries[start:lb.pos])
			}
		}
	}

	return result
}

// FilterByLevel returns log entries filtered by level
func (lb *LogBuffer) FilterByLevel(level string, limit int) []LogEntry {
	all := lb.GetRecent(lb.size)
	var filtered []LogEntry

	for _, entry := range all {
		if level == "" || entry.Level == level {
			filtered = append(filtered, entry)
			if limit > 0 && len(filtered) >= limit {
				break
			}
		}
	}

	return filtered
}
