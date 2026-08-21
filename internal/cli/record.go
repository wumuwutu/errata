package cli

import (
	"io"
	"strings"

	"github.com/wumuwutu/dejavu/internal/capture"
	"github.com/wumuwutu/dejavu/internal/config"
	"github.com/wumuwutu/dejavu/internal/fingerprint"
	"github.com/wumuwutu/dejavu/internal/hint"
	"github.com/wumuwutu/dejavu/internal/store"
)

// recordFailure fingerprints a failed command's stderr, stores it, and
// prints the restrained hit hint to hintOut when the error (or a similar
// one) was seen before. Shared by err run and the shell hook path.
//
// Recording must never break the caller: any failure is silent.
func recordFailure(commandLine, dir string, stderr []byte, cfg *config.Config, hintOut io.Writer) {
	if cfg != nil && cfg.IsIgnored(firstWord(commandLine), dir) {
		return // blacklisted: never record
	}
	if firstWord(commandLine) == "err" {
		return // never record our own commands (e.g. err run inside a hooked shell)
	}

	lang, signature, fp := fingerprint.Fingerprint(string(stderr))
	if signature == "" {
		return // not a recognizable Python/Node error: skip, never guess
	}

	dbPath, err := config.DBPath()
	if err != nil {
		return
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return
	}
	defer st.Close() //nolint:errcheck // best-effort; process exits right after

	scene := capture.SceneFor(commandLine, dir)
	rec := &store.Error{
		Fingerprint: fp,
		Signature:   signature,
		RawSample:   fingerprint.StripANSI(string(stderr)),
		Language:    lang,
		Command:     scene.Command,
		ProjectDir:  scene.Dir,
		GitCommit:   scene.GitCommit,
		Runtime:     scene.Runtime,
		OS:          scene.OS,
	}

	// Match BEFORE upserting so the hint reflects prior knowledge.
	existing, err := st.FindByFingerprint(fp)
	if err != nil {
		return
	}

	var hit *store.Error
	similar := false
	if existing != nil {
		hit = existing
		hit.Count++ // this occurrence included ("第N次")
	} else {
		if sim, _, serr := st.FindSimilar(fp, fingerprint.SimilarityThreshold); serr == nil && sim != nil {
			hit, similar = sim, true
		}
	}

	if _, _, err := st.UpsertError(rec); err != nil {
		return
	}

	if hit != nil && (cfg == nil || cfg.HintEnabled) {
		hint.Print(hintOut, hit, similar)
	}
}

// firstWord returns the basename of the first word of a command line.
func firstWord(commandLine string) string {
	fields := strings.Fields(commandLine)
	if len(fields) == 0 {
		return ""
	}
	w := fields[0]
	if i := strings.LastIndexByte(w, '/'); i >= 0 {
		w = w[i+1:]
	}
	return w
}
