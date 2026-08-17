package platform

import (
	"sort"
	"sync"
	"time"
)

type RetentionItem struct {
	Kind, ID string
	At       time.Time
}
type RetentionWorker struct {
	mu    sync.Mutex
	items []RetentionItem
}

func NewRetentionWorker() *RetentionWorker { return new(RetentionWorker) }
func (w *RetentionWorker) Add(item RetentionItem) {
	w.mu.Lock()
	w.items = append(w.items, item)
	w.mu.Unlock()
}
func (w *RetentionWorker) Run(now time.Time, age time.Duration, limit int) []RetentionItem {
	if limit <= 0 {
		return nil
	}
	cutoff := now.Add(-age)
	w.mu.Lock()
	defer w.mu.Unlock()
	sort.Slice(w.items, func(i, j int) bool { return w.items[i].At.Before(w.items[j].At) })
	var deleted []RetentionItem
	kept := w.items[:0]
	for _, item := range w.items {
		if len(deleted) < limit && item.Kind != "audit" && item.At.Before(cutoff) {
			deleted = append(deleted, item)
			continue
		}
		kept = append(kept, item)
	}
	w.items = kept
	return deleted
}
