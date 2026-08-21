// Package eval measures fingerprint quality on a labeled corpus
// (dev-guide §6.4): for each Hamming threshold it compares every pair of
// entries — predicted same-error vs annotated same-error — and reports
// precision/recall/F1. Precision is the metric that matters (§6.3:
// 宁可漏报，不可错报).
package eval

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/wumuwutu/dejavu/internal/fingerprint"
)

// Entry is one labeled corpus line.
type Entry struct {
	Raw      string `json:"raw"`
	Language string `json:"language"`
	Group    string `json:"group"`
}

// LoadCorpus reads a JSONL corpus: one Entry per line, blank lines and
// lines starting with '#' are ignored.
func LoadCorpus(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
		if e.Raw == "" || e.Group == "" {
			return nil, fmt.Errorf("%s:%d: raw and group are required", path, lineNo)
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// Metrics holds pairwise precision/recall/F1 at one Hamming threshold.
type Metrics struct {
	Threshold  int
	TP, FP, FN int
	Precision  float64
	Recall     float64
	F1         float64
}

// Evaluate fingerprints every entry (with the given normalization rules
// disabled) and scores pairwise same-error prediction at every Hamming
// threshold from 0 to maxThreshold.
//
// An entry whose stderr yields no recognizable signature never matches
// anything — counting its same-group pairs as FN, so unrecognized output
// correctly hurts recall.
func Evaluate(entries []Entry, disabled map[string]bool, maxThreshold int) []Metrics {
	fps := make([]uint64, len(entries))
	known := make([]bool, len(entries))
	for i, e := range entries {
		_, _, hex := fingerprint.FingerprintWith(e.Raw, disabled)
		if hex == "" {
			continue
		}
		h, err := strconv.ParseUint(hex, 16, 64)
		if err != nil {
			continue
		}
		fps[i] = h
		known[i] = true
	}

	// Precompute per-pair distance and ground truth once; thresholds only
	// re-aggregate.
	type pair struct {
		dist  int
		same  bool
		known bool
	}
	var pairs []pair
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			p := pair{same: entries[i].Group == entries[j].Group}
			if known[i] && known[j] {
				p.known = true
				p.dist = fingerprint.HammingDistance(fps[i], fps[j])
			}
			pairs = append(pairs, p)
		}
	}

	out := make([]Metrics, 0, maxThreshold+1)
	for t := 0; t <= maxThreshold; t++ {
		var m Metrics
		m.Threshold = t
		for _, p := range pairs {
			predicted := p.known && p.dist <= t
			switch {
			case predicted && p.same:
				m.TP++
			case predicted && !p.same:
				m.FP++
			case !predicted && p.same:
				m.FN++
			}
		}
		m.Precision = safeDiv(float64(m.TP), float64(m.TP+m.FP))
		m.Recall = safeDiv(float64(m.TP), float64(m.TP+m.FN))
		m.F1 = safeDiv(2*m.Precision*m.Recall, m.Precision+m.Recall)
		out = append(out, m)
	}
	return out
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
