package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// newUUID returns a random version 4 UUID.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// History keeps the saved downloads, so a book can be exported again without
// reading it from the site.
type History struct {
	mu      sync.Mutex
	path    string
	records []HistoryRecord
}

// NewHistory opens the history file, creating it when it is missing.
func NewHistory(path string) *History {
	h := &History{path: path}
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &h.records)
	}
	return h
}

// List returns the records, the most recently updated first.
func (h *History) List() []HistoryRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]HistoryRecord, len(h.records))
	copy(out, h.records)
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

// Count returns how many records are stored.
func (h *History) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.records)
}

// Get returns one record.
func (h *History) Get(id string) (HistoryRecord, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, record := range h.records {
		if record.ID == id {
			return record, true
		}
	}
	return HistoryRecord{}, false
}

// Save stores a record, creating it when it has no ID.
func (h *History) Save(record HistoryRecord) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	record.UpdatedAt = time.Now()
	if record.ID == "" {
		record.ID = newUUID()
		record.CreatedAt = record.UpdatedAt
	}

	found := false
	for i := range h.records {
		if h.records[i].ID == record.ID {
			h.records[i] = record
			found = true
			break
		}
	}
	if !found {
		h.records = append(h.records, record)
	}
	return record.ID, h.persistLocked()
}

// MarkExported records where a record was last written.
func (h *History) MarkExported(id, fileName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.records {
		if h.records[i].ID == id {
			now := time.Now()
			h.records[i].LastExported = &now
			h.records[i].ExportName = fileName
			h.records[i].UpdatedAt = now
			break
		}
	}
	_ = h.persistLocked()
}

// Delete removes a record.
func (h *History) Delete(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	records := h.records[:0]
	for _, record := range h.records {
		if record.ID != id {
			records = append(records, record)
		}
	}
	h.records = records
	return h.persistLocked()
}

// ByteCount is the size of the history file, shown with the app's cache.
func (h *History) ByteCount() int64 {
	info, err := os.Stat(h.path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func (h *History) persistLocked() error {
	data, err := json.Marshal(h.records)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(h.path), 0o755); err != nil {
		return err
	}
	temp := h.path + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temp, h.path)
}
