package agentnative

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

const (
	maxSessions      = 100
	maxEvents        = 10000
	maxEventBytes    = 128 << 10
	maxSessionBytes  = 8 << 20
	maxJournalBytes  = 32 << 20
	maxSnapshotBytes = 64 << 20
	maxStateBytes    = 256 << 10
	maxRequests      = 1000
	maxPromptBytes   = 100000
)

type privateDirectoryMode uint8

const (
	privateDirectoryExisting privateDirectoryMode = iota
	privateDirectoryCreate
)

type requestRecord struct {
	ID              string `json:"requestId"`
	TextHash        string `json:"textHash"`
	Status          string `json:"status"`
	TurnID          string `json:"turnId,omitempty"`
	CancelAttempted bool   `json:"cancelAttempted,omitempty"`
}

type sessionRecord struct {
	Session        Session                  `json:"session"`
	Events         []Event                  `json:"events"`
	Requests       map[string]requestRecord `json:"requests"`
	RestoreAttempt string                   `json:"restoreAttempt,omitempty"`
	ControlPending bool                     `json:"controlPending,omitempty"`
}

type snapshot struct {
	Version  int                       `json:"version"`
	Sessions map[string]*sessionRecord `json:"sessions"`
	Job      *Job                      `json:"job,omitempty"`
}

// clone copies mutable containers. Event data and adapter state are immutable
// after ingestion; readers receive separate JSON copies.
func (s snapshot) clone() snapshot {
	out := snapshot{Version: s.Version, Sessions: make(map[string]*sessionRecord, len(s.Sessions))}
	for id, record := range s.Sessions {
		r := *record
		r.Events = slices.Clone(record.Events)
		r.Requests = make(map[string]requestRecord, len(record.Requests))
		for id, request := range record.Requests {
			r.Requests[id] = request
		}
		out.Sessions[id] = &r
	}
	if s.Job != nil {
		j := *s.Job
		out.Job = &j
	}
	return out
}

type sessionStore struct {
	dir   *privateDirectory
	stamp *privateStamp
}

func openStore(dir *privateDirectory) (*sessionStore, snapshot, error) {
	store := &sessionStore{dir: dir}
	data, stamp, err := dir.read("sessions.json", maxSnapshotBytes)
	if errors.Is(err, os.ErrNotExist) {
		return store, snapshot{Version: 1, Sessions: map[string]*sessionRecord{}}, nil
	}
	if err != nil {
		return nil, snapshot{}, xerrors.Errorf("read native session archive: %w", err)
	}
	var s snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, snapshot{}, xerrors.New("invalid native session archive; nothing was resumed")
	}
	if err := validateSnapshot(s); err != nil {
		return nil, snapshot{}, err
	}
	store.stamp = stamp
	return store, s, nil
}

func (s *sessionStore) save(value snapshot) error {
	if err := validateSnapshot(value); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return xerrors.Errorf("encode native session archive: %w", err)
	}
	if len(data)+1 > maxSnapshotBytes {
		return xerrors.New("native session archive budget exhausted")
	}
	stamp, err := s.dir.write("sessions.json", append(data, '\n'), s.stamp)
	if err != nil {
		return xerrors.Errorf("persist native session archive: %w", err)
	}
	s.stamp = stamp
	return nil
}

func validUUID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && len(id) == 36 && parsed != uuid.Nil && strings.EqualFold(parsed.String(), id)
}

func validSession(s Session) bool {
	if !filepath.IsAbs(s.Cwd) || len(s.Cwd) > 4096 || strings.ContainsRune(s.Cwd, 0) || s.StartedAt < 0 ||
		(s.Mode != "human" && s.Mode != "automation") || len(s.State) > maxStateBytes ||
		(len(s.State) > 0 && !json.Valid(s.State)) || len(s.Fault) > 1024 || len(s.ActiveTurnID) > 1024 {
		return false
	}
	switch s.Harness {
	case "claude":
		return validUUID(s.NativeID) && s.ID == s.NativeID
	case "codex":
		return validUUID(s.NativeID) && s.ID == "codex:"+s.NativeID
	case "opencode":
		return strings.HasPrefix(s.NativeID, "ses_") && len(s.NativeID) > 4 && len(s.NativeID) < 256 &&
			!strings.ContainsAny(s.NativeID, " /\\\r\n\t\x00") && s.ID == "opencode:"+s.NativeID
	default:
		return false
	}
}

func validateSnapshot(s snapshot) error {
	invalid := xerrors.New("invalid or oversized native session archive; nothing was resumed")
	if s.Version != 1 || s.Sessions == nil || len(s.Sessions) > maxSessions {
		return invalid
	}
	total := 0
	for id, r := range s.Sessions {
		if r == nil || r.Session.ID != id || !validSession(r.Session) || len(r.Events) > maxEvents || len(r.Requests) > maxRequests ||
			r.Session.HistoryStartCursor > r.Session.Cursor || (r.RestoreAttempt != "" && !validUUID(r.RestoreAttempt)) {
			return invalid
		}
		seq := r.Session.HistoryStartCursor
		bytes := 0
		for _, event := range r.Events {
			data, err := json.Marshal(event)
			if err != nil || event.Seq == 0 || event.Seq != seq+1 || event.Type == "" || len(data) > maxEventBytes || event.Data == nil {
				return invalid
			}
			seq = event.Seq
			bytes += len(data)
		}
		if seq != r.Session.Cursor || bytes > maxSessionBytes {
			return invalid
		}
		total += bytes
		for id, request := range r.Requests {
			if !validUUID(id) || request.ID != id || len(request.TextHash) != sha256.Size*2 {
				return invalid
			}
			if _, err := hex.DecodeString(request.TextHash); err != nil {
				return invalid
			}
			switch request.Status {
			case "attempted", "running", "completed", "rejected", "canceled", "failed", "unknown":
			default:
				return invalid
			}
		}
		if r.Session.ActiveRequestID != "" {
			if _, ok := r.Requests[r.Session.ActiveRequestID]; !ok {
				return invalid
			}
		}
	}
	if total > maxJournalBytes {
		return invalid
	}
	if s.Job != nil && !validJob(*s.Job, s) {
		return invalid
	}
	return nil
}

func boundJournal(s *snapshot) {
	total := 0
	bytes := make(map[string]int, len(s.Sessions))
	for id, r := range s.Sessions {
		for _, event := range r.Events {
			data, _ := json.Marshal(event)
			bytes[id] += len(data)
		}
		for len(r.Events) > maxEvents || bytes[id] > maxSessionBytes {
			bytes[id] -= evictEvent(r)
		}
		total += bytes[id]
	}
	// Evict from the largest journal first, with stable ties. Tombstones and
	// adapter receipts are never evicted with display history.
	for total > maxJournalBytes {
		largest := ""
		for id, size := range bytes {
			if size > bytes[largest] || (size == bytes[largest] && id < largest) {
				largest = id
			}
		}
		n := evictEvent(s.Sessions[largest])
		bytes[largest] -= n
		total -= n
	}
}

func evictEvent(r *sessionRecord) int {
	e := r.Events[0]
	data, _ := json.Marshal(e)
	r.Session.HistoryStartCursor = e.Seq
	r.Events[0] = Event{}
	r.Events = r.Events[1:]
	return len(data)
}

func textHash(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}
