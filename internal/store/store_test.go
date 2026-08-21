package store

import (
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "dejavu.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sample(fp string) *Error {
	return &Error{
		Fingerprint: fp,
		Signature:   "TypeError: unsupported operand type(s) for +: <VAL> and <VAL>",
		RawSample:   "Traceback ...\nTypeError: unsupported operand type(s) for +: 'int' and 'str'",
		Language:    "python",
		Command:     "python train.py",
		ProjectDir:  "/home/x/proj",
		GitCommit:   "abc1234",
		Runtime:     "Python 3.12.4",
		OS:          "linux/amd64",
	}
}

func TestUpsertNewThenReoccurrence(t *testing.T) {
	s := openTemp(t)

	id, isNew, err := s.UpsertError(sample("00000000000000ff"))
	if err != nil || !isNew || id == 0 {
		t.Fatalf("first upsert: id=%d isNew=%v err=%v", id, isNew, err)
	}

	id2, isNew2, err := s.UpsertError(sample("00000000000000ff"))
	if err != nil || isNew2 || id2 != id {
		t.Fatalf("second upsert: id=%d isNew=%v err=%v", id2, isNew2, err)
	}

	e, err := s.Get(id)
	if err != nil || e == nil {
		t.Fatalf("Get: %v", err)
	}
	if e.Count != 2 {
		t.Fatalf("count = %d, want 2", e.Count)
	}
	if !e.LastSeen.After(e.FirstSeen) && !e.LastSeen.Equal(e.FirstSeen) {
		t.Fatalf("last_seen %v before first_seen %v", e.LastSeen, e.FirstSeen)
	}
	if e.Pending != "pending" {
		t.Fatalf("pending status = %q", e.Pending)
	}
	if e.GitCommit != "abc1234" || e.Runtime != "Python 3.12.4" || e.OS != "linux/amd64" {
		t.Fatalf("scene lost: %+v", e)
	}
}

func TestFindByFingerprintMiss(t *testing.T) {
	s := openTemp(t)
	e, err := s.FindByFingerprint("0000000000000000")
	if err != nil || e != nil {
		t.Fatalf("want (nil,nil), got (%v,%v)", e, err)
	}
}

func TestLatestPendingAndAddFix(t *testing.T) {
	s := openTemp(t)
	if _, isNew, _ := s.UpsertError(sample("0000000000000001")); !isNew {
		t.Fatal("want new")
	}
	id2, _, _ := s.UpsertError(sample("0000000000000002"))

	p, err := s.LatestPending()
	if err != nil || p == nil {
		t.Fatalf("LatestPending: %v", err)
	}
	if p.ErrorID != id2 {
		t.Fatalf("latest pending error_id = %d, want %d", p.ErrorID, id2)
	}

	if err := s.AddFix(id2, "conda deactivate 后重装"); err != nil {
		t.Fatalf("AddFix: %v", err)
	}

	e, _ := s.Get(id2)
	if e.Solution != "conda deactivate 后重装" {
		t.Fatalf("solution = %q", e.Solution)
	}
	if e.Pending != "resolved" {
		t.Fatalf("pending status = %q, want resolved", e.Pending)
	}

	// The other error is still pending and now the latest one.
	p, _ = s.LatestPending()
	if p == nil || p.ErrorID == id2 {
		t.Fatalf("latest pending after fix: %+v", p)
	}
}

func TestFindSimilar(t *testing.T) {
	s := openTemp(t)
	// 0x...f0 vs 0x...ff differ in 4 bits; 0x...00 differs in 8 bits.
	if _, _, err := s.UpsertError(sample("00000000000000f0")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.UpsertError(sample("0000000000000000")); err != nil {
		t.Fatal(err)
	}

	hit, dist, err := s.FindSimilar("00000000000000ff", 6)
	if err != nil {
		t.Fatal(err)
	}
	if hit == nil || dist != 4 || hit.Fingerprint != "00000000000000f0" {
		t.Fatalf("FindSimilar: hit=%+v dist=%d", hit, dist)
	}

	hit, dist, err = s.FindSimilar("00000000000000ff", 3)
	if err != nil {
		t.Fatal(err)
	}
	if hit != nil || dist != 0 {
		t.Fatalf("FindSimilar with threshold 3: hit=%+v dist=%d, want none", hit, dist)
	}
}

func TestFTS5IndexMaintained(t *testing.T) {
	s := openTemp(t)
	id, _, err := s.UpsertError(sample("00000000000000aa"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddFix(id, "reinstall torch with pip"); err != nil {
		t.Fatal(err)
	}
	var found int64
	err = s.db.QueryRow(
		`SELECT rowid FROM errors_fts WHERE errors_fts MATCH 'torch'`).Scan(&found)
	if err != nil {
		t.Fatalf("FTS query: %v", err)
	}
	if found != id {
		t.Fatalf("FTS rowid = %d, want %d", found, id)
	}
}

func TestUnknownID(t *testing.T) {
	s := openTemp(t)
	e, err := s.Get(999)
	if err != nil || e != nil {
		t.Fatalf("want (nil,nil), got (%v,%v)", e, err)
	}
}
