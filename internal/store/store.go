// Package store persists error records, fixes and pending state in a local
// SQLite database (pure Go driver, no CGO). Local-first: nothing ever
// leaves the machine (dev-guide §9).
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite" // pure-Go sqlite driver (database/sql name "sqlite")

	"github.com/wumuwutu/dejavu/internal/fingerprint"
)

const timeLayout = time.RFC3339Nano

// Error is one error record with its scene, timeline and (optional) fix.
type Error struct {
	ID          int64
	Fingerprint string
	Signature   string
	RawSample   string
	Language    string
	Command     string
	ProjectDir  string
	GitCommit   string
	Runtime     string
	OS          string
	CreatedAt   time.Time
	FirstSeen   time.Time
	LastSeen    time.Time
	Count       int
	Solution    string // latest fix, "" if none
	Pending     string // latest pending status, "" if none
}

// Pending is an unresolved-error queue entry.
type Pending struct {
	ID         int64
	ErrorID    int64
	Signature  string
	DetectedAt time.Time
	Status     string
}

// Store wraps the SQLite handle.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the database at path and ensures the schema.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// WAL + busy_timeout: two terminals (hook-event, err run) can write
	// concurrently; without them a second writer gets an immediate
	// SQLITE_BUSY and the capture is silently lost.
	dsn := url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)",
	}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// FindByFingerprint returns the record with this exact fingerprint, or
// (nil, nil) if unknown.
func (s *Store) FindByFingerprint(fp string) (*Error, error) {
	row := s.db.QueryRow(selectError+` WHERE e.fingerprint = ?`, fp)
	e, err := scanError(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

// FindSimilar returns the record whose fingerprint is closest to fp within
// maxDist Hamming bits (exclusive of exact matches), plus the distance.
// (nil, 0, nil) when nothing qualifies. MVP scans all rows; the library is
// a personal error log, not a fleet-scale store.
func (s *Store) FindSimilar(fp string, maxDist int) (*Error, int, error) {
	target, err := strconv.ParseUint(fp, 16, 64)
	if err != nil {
		return nil, 0, fmt.Errorf("bad fingerprint %q: %w", fp, err)
	}
	rows, err := s.db.Query(`SELECT id, fingerprint FROM errors WHERE fingerprint != ?`, fp)
	if err != nil {
		return nil, 0, err
	}
	type cand struct {
		id   int64
		dist int
	}
	best := cand{id: -1, dist: maxDist + 1}
	for rows.Next() {
		var id int64
		var hex string
		if err := rows.Scan(&id, &hex); err != nil {
			rows.Close()
			return nil, 0, err
		}
		h, err := strconv.ParseUint(hex, 16, 64)
		if err != nil {
			continue
		}
		if d := fingerprint.HammingDistance(target, h); d < best.dist {
			best = cand{id: id, dist: d}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()
	if best.id < 0 || best.dist > maxDist {
		return nil, 0, nil
	}
	e, err := s.Get(best.id)
	return e, best.dist, err
}

// UpsertError inserts a new error record (with a pending entry) or, if the
// fingerprint is already known, bumps last_seen/count and refreshes the
// raw sample. It reports whether the record is new.
func (s *Store) UpsertError(e *Error) (id int64, isNew bool, err error) {
	now := time.Now()
	existing, err := s.FindByFingerprint(e.Fingerprint)
	if err != nil {
		return 0, false, err
	}
	if existing != nil {
		_, err = s.db.Exec(
			`UPDATE errors SET last_seen = ?, count = count + 1, raw_sample = ? WHERE id = ?`,
			now.Format(timeLayout), e.RawSample, existing.ID)
		return existing.ID, false, err
	}
	res, err := s.db.Exec(
		`INSERT INTO errors (fingerprint, signature, raw_sample, language, command,
		  project_dir, git_commit, runtime, os, created_at, first_seen, last_seen, count)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,1)`,
		e.Fingerprint, e.Signature, e.RawSample, e.Language, e.Command,
		e.ProjectDir, e.GitCommit, e.Runtime, e.OS,
		now.Format(timeLayout), now.Format(timeLayout), now.Format(timeLayout))
	if err != nil {
		return 0, false, err
	}
	id, err = res.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	if _, err := s.db.Exec(
		`INSERT INTO pending (error_id, detected_at, status) VALUES (?,?,'pending')`,
		id, now.Format(timeLayout)); err != nil {
		return 0, false, err
	}
	if _, err := s.db.Exec(
		`INSERT INTO errors_fts(rowid, signature, solution) VALUES (?,?,'')`,
		id, e.Signature); err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// LatestPending returns the most recently detected unresolved error, or
// (nil, nil) if there is none.
func (s *Store) LatestPending() (*Pending, error) {
	row := s.db.QueryRow(
		`SELECT p.id, p.error_id, e.signature, p.detected_at, p.status
		 FROM pending p JOIN errors e ON e.id = p.error_id
		 WHERE p.status = 'pending'
		 ORDER BY p.detected_at DESC, p.id DESC LIMIT 1`)
	var p Pending
	var detected string
	if err := row.Scan(&p.ID, &p.ErrorID, &p.Signature, &detected, &p.Status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	p.DetectedAt = parseTime(detected)
	return &p, nil
}

// AddFix records a user-confirmed solution for an error, marks its pending
// entries resolved, and updates the full-text index.
func (s *Store) AddFix(errorID int64, solution string) error {
	now := time.Now().Format(timeLayout)
	if _, err := s.db.Exec(
		`INSERT INTO fixes (error_id, solution, created_at) VALUES (?,?,?)`,
		errorID, solution, now); err != nil {
		return err
	}
	if _, err := s.db.Exec(
		`UPDATE pending SET status = 'resolved' WHERE error_id = ? AND status = 'pending'`,
		errorID); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE errors_fts SET solution = ? WHERE rowid = ?`, solution, errorID)
	return err
}

// PendingItem is a pending (unresolved) error with its record details.
type PendingItem struct {
	ErrorID    int64
	Signature  string
	Language   string
	Count      int
	FirstSeen  time.Time
	LastSeen   time.Time
	DetectedAt time.Time
}

// ListPending returns all unresolved errors, most recently detected first.
// Archived entries are excluded (not deleted, just out of view).
func (s *Store) ListPending() ([]PendingItem, error) {
	rows, err := s.db.Query(
		`SELECT p.error_id, e.signature, e.language, e.count,
		        e.first_seen, e.last_seen, p.detected_at
		 FROM pending p JOIN errors e ON e.id = p.error_id
		 WHERE p.status = 'pending'
		 ORDER BY p.detected_at DESC, p.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingItem
	for rows.Next() {
		var it PendingItem
		var first, last, detected string
		if err := rows.Scan(&it.ErrorID, &it.Signature, &it.Language, &it.Count,
			&first, &last, &detected); err != nil {
			return nil, err
		}
		it.FirstSeen = parseTime(first)
		it.LastSeen = parseTime(last)
		it.DetectedAt = parseTime(detected)
		out = append(out, it)
	}
	return out, rows.Err()
}

// RecordRate returns how many errors have at least one fix out of the
// total — the product health metric from dev-guide §7.5.
func (s *Store) RecordRate() (resolved, total int, err error) {
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM errors`).Scan(&total); err != nil {
		return 0, 0, err
	}
	err = s.db.QueryRow(
		`SELECT COUNT(DISTINCT error_id) FROM fixes`).Scan(&resolved)
	return resolved, total, err
}

// ArchiveStalePending marks pending entries detected before cutoff as
// archived. Nothing is deleted; archived errors simply leave the pending
// queue (dev-guide §7.5). Returns the number archived.
func (s *Store) ArchiveStalePending(cutoff time.Time) (int64, error) {
	rows, err := s.db.Query(
		`SELECT id, detected_at FROM pending WHERE status = 'pending'`)
	if err != nil {
		return 0, err
	}
	var stale []int64
	for rows.Next() {
		var id int64
		var detected string
		if err := rows.Scan(&id, &detected); err != nil {
			rows.Close()
			return 0, err
		}
		if t := parseTime(detected); !t.IsZero() && t.Before(cutoff) {
			stale = append(stale, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, id := range stale {
		if _, err := s.db.Exec(
			`UPDATE pending SET status = 'archived' WHERE id = ?`, id); err != nil {
			return int64(len(stale)), err
		}
	}
	return int64(len(stale)), nil
}

const selectError = `
SELECT e.id,
       COALESCE(e.fingerprint, ''), COALESCE(e.signature, ''),
       COALESCE(e.raw_sample, ''), COALESCE(e.language, ''),
       COALESCE(e.command, ''), COALESCE(e.project_dir, ''),
       COALESCE(e.git_commit, ''), COALESCE(e.runtime, ''), COALESCE(e.os, ''),
       COALESCE(e.created_at, ''), COALESCE(e.first_seen, ''),
       COALESCE(e.last_seen, ''), COALESCE(e.count, 0),
       COALESCE((SELECT f.solution FROM fixes f WHERE f.error_id = e.id
                 ORDER BY f.id DESC LIMIT 1), ''),
       COALESCE((SELECT p.status FROM pending p WHERE p.error_id = e.id
                 ORDER BY p.id DESC LIMIT 1), '')
FROM errors e`

// Get returns one error record by id, or (nil, nil) if unknown.
func (s *Store) Get(id int64) (*Error, error) {
	row := s.db.QueryRow(selectError+` WHERE e.id = ?`, id)
	e, err := scanError(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanError(row scannable) (*Error, error) {
	var e Error
	var created, first, last string
	err := row.Scan(&e.ID, &e.Fingerprint, &e.Signature, &e.RawSample, &e.Language,
		&e.Command, &e.ProjectDir, &e.GitCommit, &e.Runtime, &e.OS,
		&created, &first, &last, &e.Count, &e.Solution, &e.Pending)
	if err != nil {
		return nil, err
	}
	e.CreatedAt = parseTime(created)
	e.FirstSeen = parseTime(first)
	e.LastSeen = parseTime(last)
	return &e, nil
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(timeLayout, s)
	return t
}
