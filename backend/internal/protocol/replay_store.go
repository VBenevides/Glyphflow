package protocol

import (
	"bufio"
	"errors"
	"os"
	"sync"
)

type ReplayLog struct {
	mu   sync.Mutex
	path string
	seen map[string]struct{}
}

func OpenReplayLog(path string) (*ReplayLog, error) {
	log := &ReplayLog{path: path, seen: make(map[string]struct{})}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return log, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if scanner.Text() != "" {
			log.seen[scanner.Text()] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return log, nil
}

func (l *ReplayLog) Accept(id string) error {
	if id == "" {
		return errors.New("message ID is empty")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.seen[id]; ok {
		return errors.New("message replay detected")
	}
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	if _, err = file.WriteString(id + "\n"); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	l.seen[id] = struct{}{}
	return nil
}
