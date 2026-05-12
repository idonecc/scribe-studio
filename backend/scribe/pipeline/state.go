package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/autogame-17/scribe-studio/backend/scribe/runtime"
)

// State is the persistent map of Jobs keyed by taskID, plus a "seen"
// set for the watcher. Protected by a single mutex — writes are rare
// (seconds apart) so contention isn't a concern.
type State struct {
	mu          sync.Mutex
	path        string
	ScannedOnce bool            `json:"scannedOnce"`
	SeenIDs     map[string]bool `json:"seenIDs"`
	Jobs        map[string]Job  `json:"jobs"`
}

func stateFilePath() (string, error) {
	dir, err := runtime.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pipeline.json"), nil
}

// LoadState reads the on-disk state, creating an empty one if missing.
func LoadState() (*State, error) {
	p, err := stateFilePath()
	if err != nil {
		return nil, err
	}
	s := &State{
		path:    p,
		SeenIDs: map[string]bool{},
		Jobs:    map[string]Job{},
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(raw, s); err != nil {
		// Corrupt state: log+discard rather than wedge the pipeline.
		return s, nil
	}
	if s.SeenIDs == nil {
		s.SeenIDs = map[string]bool{}
	}
	if s.Jobs == nil {
		s.Jobs = map[string]Job{}
	}
	s.path = p
	return s, nil
}

// Save writes atomically via write-to-tmp + rename. Callers should
// tolerate a returned error (we log but don't crash).
func (s *State) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *State) Seen(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.SeenIDs[id]
}

func (s *State) MarkSeen(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SeenIDs[id] = true
}

func (s *State) HasEverScanned() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ScannedOnce
}

func (s *State) MarkScanned() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ScannedOnce = true
}

func (s *State) Get(id string) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.Jobs[id]
	return j, ok
}

func (s *State) Upsert(j Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Jobs[j.TaskID] = j
}

// Snapshot returns jobs sorted by updatedAt desc for the UI.
func (s *State) Snapshot() []Job {
	s.mu.Lock()
	out := make([]Job, 0, len(s.Jobs))
	for _, j := range s.Jobs {
		out = append(out, j)
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out
}
