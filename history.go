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
//
// It is an index plus one file per book. The list reads only the index, so
// opening the history never deserializes a whole book, and saving a chapter
// rewrites one record rather than the whole library.
//
// The on-disk shape:
//
//	<dir>/index.json      the summaries, newest last
//	<dir>/books/<id>.json one record, chapters included
type History struct {
	mu      sync.Mutex
	dir     string
	index   []HistorySummary
	loaded  bool
	loadErr error
}

// NewHistory opens the history directory, creating it when it is missing.
func NewHistory(dir string) *History {
	h := &History{dir: dir}
	h.load()
	return h
}

// indexFile is the index's path.
func (h *History) indexFile() string { return filepath.Join(h.dir, "index.json") }

// bookFile is one record's path.
func (h *History) bookFile(id string) string { return filepath.Join(h.dir, "books", id+".json") }

// load reads the index, migrating the old single-file history once.
func (h *History) load() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.loadLocked()
}

func (h *History) loadLocked() {
	if h.loaded {
		return
	}
	h.loaded = true

	if data, err := os.ReadFile(h.indexFile()); err == nil {
		if err := json.Unmarshal(data, &h.index); err != nil {
			h.loadErr = err
			h.index = nil
		}
		return
	}

	// The old shape was one history.json holding every record with its
	// chapters. Migrate it once, keeping the original as .bak.
	legacy := filepath.Join(h.dir, "history.json")
	data, err := os.ReadFile(legacy)
	if err != nil {
		return
	}
	var records []HistoryRecord
	if err := json.Unmarshal(data, &records); err != nil {
		h.loadErr = err
		return
	}
	for _, record := range records {
		if record.ID == "" {
			record.ID = newUUID()
		}
		if record.CreatedAt.IsZero() {
			record.CreatedAt = time.Now()
		}
		if record.UpdatedAt.IsZero() {
			record.UpdatedAt = record.CreatedAt
		}
		if err := h.writeRecordLocked(record); err != nil {
			h.loadErr = err
			continue
		}
		h.index = append(h.index, record.Summary())
	}
	_ = h.persistIndexLocked()
	_ = os.Rename(legacy, legacy+".bak")
}

// List returns the summaries, the most recently updated first.
func (h *History) List() []HistorySummary {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.loadLocked()
	out := make([]HistorySummary, len(h.index))
	copy(out, h.index)
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

// Count returns how many records are stored.
func (h *History) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.loadLocked()
	return len(h.index)
}

// Get returns one record, chapters included.
func (h *History) Get(id string) (HistoryRecord, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.loadLocked()
	data, err := os.ReadFile(h.bookFile(id))
	if err != nil {
		return HistoryRecord{}, false
	}
	var record HistoryRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return HistoryRecord{}, false
	}
	return record, true
}

// Save stores a record, creating it when it has no ID. A record for a book
// already in the history replaces that one, so downloading the same book twice
// leaves one entry rather than two.
func (h *History) Save(record HistoryRecord) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.loadLocked()

	record.UpdatedAt = time.Now()
	if record.ID == "" {
		record.ID = h.idForLocked(record.SourceURL)
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = record.UpdatedAt
	}

	if err := h.writeRecordLocked(record); err != nil {
		return "", err
	}

	summary := record.Summary()
	replaced := false
	for i := range h.index {
		if h.index[i].ID == record.ID {
			h.index[i] = summary
			replaced = true
			break
		}
	}
	if !replaced {
		h.index = append(h.index, summary)
	}
	return record.ID, h.persistIndexLocked()
}

// idForLocked is the ID of the record for a book: the one already stored for
// that address, or a new one.
func (h *History) idForLocked(sourceURL string) string {
	if sourceURL != "" {
		for _, summary := range h.index {
			if summary.SourceURL == sourceURL {
				return summary.ID
			}
		}
	}
	return newUUID()
}

// MarkExported records where a record was last written.
func (h *History) MarkExported(id, fileName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.loadLocked()

	now := time.Now()
	for i := range h.index {
		if h.index[i].ID == id {
			h.index[i].LastExported = &now
			h.index[i].ExportName = fileName
			h.index[i].UpdatedAt = now
			break
		}
	}
	if record, ok := h.getLocked(id); ok {
		record.LastExported = &now
		record.ExportName = fileName
		record.UpdatedAt = now
		_ = h.writeRecordLocked(record)
	}
	_ = h.persistIndexLocked()
}

// Delete removes a record and its file.
func (h *History) Delete(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.loadLocked()

	kept := h.index[:0]
	for _, summary := range h.index {
		if summary.ID != id {
			kept = append(kept, summary)
		}
	}
	h.index = kept
	if err := os.Remove(h.bookFile(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return h.persistIndexLocked()
}

// ByteCount is the size of the history directory, shown with the app's cache.
func (h *History) ByteCount() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	var total int64
	_ = filepath.WalkDir(h.dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, err := entry.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// getLocked reads a record. The caller holds the lock.
func (h *History) getLocked(id string) (HistoryRecord, bool) {
	data, err := os.ReadFile(h.bookFile(id))
	if err != nil {
		return HistoryRecord{}, false
	}
	var record HistoryRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return HistoryRecord{}, false
	}
	return record, true
}

// writeRecordLocked writes one record's file. The caller holds the lock.
func (h *History) writeRecordLocked(record HistoryRecord) error {
	if err := os.MkdirAll(filepath.Join(h.dir, "books"), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return writeFileAtomic(h.bookFile(record.ID), data)
}

// persistIndexLocked writes the index. The caller holds the lock.
func (h *History) persistIndexLocked() error {
	if err := os.MkdirAll(h.dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(h.index)
	if err != nil {
		return err
	}
	return writeFileAtomic(h.indexFile(), data)
}

// writeFileAtomic replaces a file in one step, so a crash mid-write cannot
// leave a half-written history behind.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
