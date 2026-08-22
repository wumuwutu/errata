package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
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

func TestListPendingAndRecordRate(t *testing.T) {
	s := openTemp(t)
	id1, _, _ := s.UpsertError(sample("00000000000000a1"))
	id2, _, _ := s.UpsertError(sample("00000000000000a2"))

	items, err := s.ListPending()
	if err != nil || len(items) != 2 {
		t.Fatalf("ListPending: %v, %d items", err, len(items))
	}
	if items[0].ErrorID != id2 {
		t.Fatalf("most recent first: got %d, want %d", items[0].ErrorID, id2)
	}
	if items[0].Language != "python" || items[0].Signature == "" {
		t.Fatalf("item incomplete: %+v", items[0])
	}

	resolved, total, err := s.RecordRate()
	if err != nil || resolved != 0 || total != 2 {
		t.Fatalf("RecordRate: %d/%d err=%v", resolved, total, err)
	}

	if err := s.AddFix(id1, "fixed it"); err != nil {
		t.Fatal(err)
	}
	resolved, total, _ = s.RecordRate()
	if resolved != 1 || total != 2 {
		t.Fatalf("RecordRate after fix: %d/%d", resolved, total)
	}
	items, _ = s.ListPending()
	if len(items) != 1 || items[0].ErrorID != id2 {
		t.Fatalf("ListPending after fix: %+v", items)
	}
}

func TestArchiveStalePending(t *testing.T) {
	s := openTemp(t)
	idNew, _, _ := s.UpsertError(sample("00000000000000b1"))
	idOld, _, _ := s.UpsertError(sample("00000000000000b2"))

	// Age the second error's pending entry by 60 days.
	old := time.Now().AddDate(0, 0, -60).Format(timeLayout)
	if _, err := s.db.Exec(
		`UPDATE pending SET detected_at = ? WHERE error_id = ?`, old, idOld); err != nil {
		t.Fatal(err)
	}

	archived, err := s.ArchiveStalePending(time.Now().AddDate(0, 0, -30))
	if err != nil || archived != 1 {
		t.Fatalf("ArchiveStalePending: archived=%d err=%v", archived, err)
	}

	items, _ := s.ListPending()
	if len(items) != 1 || items[0].ErrorID != idNew {
		t.Fatalf("after archive: %+v", items)
	}
	// Archived, not deleted: the error record itself survives.
	e, _ := s.Get(idOld)
	if e == nil || e.Pending != "archived" {
		t.Fatalf("archived record: %+v", e)
	}
	// Re-running archives nothing.
	archived, _ = s.ArchiveStalePending(time.Now().AddDate(0, 0, -30))
	if archived != 0 {
		t.Fatalf("re-archive: %d", archived)
	}
}

func TestRecentPendingInDir(t *testing.T) {
	s := openTemp(t)
	now := time.Now()
	window := 5 * time.Minute
	remind := 24 * time.Hour

	e := sample("00000000000000e1")
	e.ProjectDir = "/proj"
	id, _, _ := s.UpsertError(e)

	// Fresh pending error in the same dir qualifies.
	got, err := s.RecentPendingInDir("/proj", now, window, remind)
	if err != nil || len(got) != 1 || got[0].ID != id {
		t.Fatalf("want [error %d], got %+v err=%v", id, got, err)
	}

	// Different directory does not.
	if got, _ := s.RecentPendingInDir("/elsewhere", now, window, remind); len(got) != 0 {
		t.Fatalf("wrong dir matched: %+v", got)
	}

	// After a reminder, the same error stays quiet for remindEvery...
	if err := s.MarkReminded(id, now); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.RecentPendingInDir("/proj", now, window, remind); len(got) != 0 {
		t.Fatal("reminded error must stay quiet")
	}
	// ...and speaks again once remindEvery has passed (huge window to
	// isolate the remind logic from last_seen aging).
	if got, _ := s.RecentPendingInDir("/proj", now.Add(2*remind), 10000*time.Hour, remind); len(got) != 1 {
		t.Fatal("after remindEvery the error should qualify again")
	}

	// Outside the success window it no longer qualifies.
	if got, _ := s.RecentPendingInDir("/proj", now.Add(time.Hour), window, remind); len(got) != 0 {
		t.Fatal("stale pending must not match the success window")
	}

	// Resolved errors never qualify.
	if err := s.AddFix(id, "done"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.RecentPendingInDir("/proj", now, window, remind); len(got) != 0 {
		t.Fatal("resolved error must not match")
	}
}

func TestStats(t *testing.T) {
	s := openTemp(t)
	now := time.Now()

	e1 := sample("0000000000000011")
	e1.ProjectDir = "/proj-a"
	if _, _, err := s.UpsertError(e1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.UpsertError(e1); err != nil { // count=2
		t.Fatal(err)
	}
	e2 := sample("0000000000000012")
	e2.Language = "node"
	e2.ProjectDir = "/proj-b"
	if _, _, err := s.UpsertError(e2); err != nil {
		t.Fatal(err)
	}
	// An old error outside the weekly window.
	e3 := sample("0000000000000013")
	id3, _, _ := s.UpsertError(e3)
	old := now.Add(-60 * 24 * time.Hour).Format(timeLayout)
	if _, err := s.db.Exec(`UPDATE errors SET created_at = ? WHERE id = ?`, old, id3); err != nil {
		t.Fatal(err)
	}
	if err := s.AddFix(1, "fix"); err != nil {
		t.Fatal(err)
	}

	st, err := s.Stats(now, 5, 5)
	if err != nil {
		t.Fatal(err)
	}
	if st.Total != 3 || st.Resolved != 1 {
		t.Fatalf("total/resolved = %d/%d, want 3/1", st.Total, st.Resolved)
	}
	if len(st.ByLanguage) != 2 {
		t.Fatalf("ByLanguage = %v", st.ByLanguage)
	}
	// proj-a has 2 occurrences, the others 1 each -> proj-a first.
	if st.ByProject[0].Label != "/proj-a" || st.ByProject[0].N != 2 {
		t.Fatalf("ByProject[0] = %+v", st.ByProject[0])
	}
	if st.TopRepeated[0].N != 2 {
		t.Fatalf("TopRepeated[0] = %+v", st.TopRepeated[0])
	}
	total := 0
	for _, n := range st.WeeklyNew {
		total += n
	}
	if total != 2 {
		t.Fatalf("weekly new total = %d, want 2 (old error excluded)", total)
	}
}

func TestDeleteError(t *testing.T) {
	s := openTemp(t)
	id, _, err := s.UpsertError(sample("00000000000000aa"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddFix(id, "the fix"); err != nil {
		t.Fatal(err)
	}

	ok, err := s.DeleteError(id)
	if err != nil || !ok {
		t.Fatalf("DeleteError: ok=%v err=%v", ok, err)
	}
	if e, _ := s.Get(id); e != nil {
		t.Fatalf("error %d still present", id)
	}
	items, _ := s.ListPending()
	if len(items) != 0 {
		t.Fatalf("pending row survived delete: %+v", items)
	}
	resolved, total, _ := s.RecordRate()
	if resolved != 0 || total != 0 {
		t.Fatalf("fix row survived delete: %d/%d", resolved, total)
	}

	ok, err = s.DeleteError(id)
	if err != nil || ok {
		t.Fatalf("second delete: ok=%v err=%v", ok, err)
	}
}

func TestClearAll(t *testing.T) {
	s := openTemp(t)
	for _, fp := range []string{"00000000000000a1", "00000000000000a2", "00000000000000a3"} {
		if _, _, err := s.UpsertError(sample(fp)); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.ClearAll()
	if err != nil || n != 3 {
		t.Fatalf("ClearAll = %d, %v; want 3", n, err)
	}
	if items, _ := s.ListAll(); len(items) != 0 {
		t.Fatalf("records survived clear: %d", len(items))
	}
	// After a clear the library is pristine: ids start at 1 again.
	id, _, err := s.UpsertError(sample("00000000000000b1"))
	if err != nil || id != 1 {
		t.Fatalf("first id after clear = %d, %v; want 1", id, err)
	}
}

// TestMigration3Autoincrement: a v0.1.5 (schema v2) database upgrades in
// place, data intact, and ids are never reused after deleting the max id.
func TestMigration3Autoincrement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dejavu.db")

	// Build a schema-v2 database by hand (migrations 1+2 only).
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(migrations[i]); err != nil {
			t.Fatalf("legacy migration %d: %v", i+1, err)
		}
	}
	if _, err := db.Exec(`DELETE FROM schema_version`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_version (version) VALUES (2)`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Format(timeLayout)
	for i, fp := range []string{"aaaaaaaaaaaaaaa1", "aaaaaaaaaaaaaaa2"} {
		if _, err := db.Exec(
			`INSERT INTO errors (fingerprint, signature, language, command, project_dir,
			  created_at, first_seen, last_seen, count) VALUES (?,?,?,?,?,?,?,?,1)`,
			fp, "TypeError: legacy", "python", "python app.py", "/proj", now, now, now); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(
			`INSERT INTO pending (error_id, detected_at, status) VALUES (?,?,'pending')`,
			i+1, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Open upgrades to v3; both records survive with their ids.
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open upgrade: %v", err)
	}
	defer s.Close()
	items, err := s.ListAll()
	if err != nil || len(items) != 2 {
		t.Fatalf("after upgrade: %d items, %v", len(items), err)
	}
	if items[0].Signature != "TypeError: legacy" || items[0].ID == 0 {
		t.Fatalf("data damaged: %+v", items[0])
	}

	// Delete the max id, insert a new error: the id must NOT be reused.
	if ok, err := s.DeleteError(2); err != nil || !ok {
		t.Fatalf("delete max id: ok=%v err=%v", ok, err)
	}
	id, _, err := s.UpsertError(sample("aaaaaaaaaaaaaaa3"))
	if err != nil {
		t.Fatal(err)
	}
	if id != 3 {
		t.Fatalf("new id = %d, want 3 (no reuse)", id)
	}
}
