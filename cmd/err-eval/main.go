// err-eval measures the fingerprint pipeline against a labeled corpus
// (dev-guide §6.4). It is a separate binary so the eval toolchain never
// pollutes the main CLI.
//
// Usage:
//
//	err-eval -corpus eval/corpus.jsonl
//	err-eval -disable val,num        # ablation: run without these rules
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/wumuwutu/dejavu/internal/eval"
	"github.com/wumuwutu/dejavu/internal/fingerprint"
)

func main() {
	corpusPath := flag.String("corpus", "eval/corpus.jsonl", "JSONL corpus path")
	disable := flag.String("disable", "", "comma-separated normalization rules to disable (ablation); any of: "+strings.Join(fingerprint.RuleNames, ","))
	maxT := flag.Int("max-threshold", 10, "maximum Hamming threshold to evaluate")
	flag.Parse()

	disabled, err := parseDisabled(*disable)
	if err != nil {
		fmt.Fprintln(os.Stderr, "err-eval:", err)
		os.Exit(1)
	}

	entries, err := eval.LoadCorpus(*corpusPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "err-eval:", err)
		os.Exit(1)
	}

	groups := map[string]int{}
	recognized := 0
	for _, e := range entries {
		groups[e.Group]++
		if _, _, hex := fingerprint.FingerprintWith(e.Raw, disabled); hex != "" {
			recognized++
		}
	}
	fmt.Printf("corpus: %d entries, %d error groups, %d recognized (python+node precise, generic fallback)\n",
		len(entries), len(groups), recognized)
	if len(disabled) > 0 {
		fmt.Printf("ablation: disabled rules: %s\n", *disable)
	}
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "threshold\tTP\tFP\tFN\tprecision\trecall\tF1")
	for _, m := range eval.Evaluate(entries, disabled, *maxT) {
		fmt.Fprintf(w, "%d\t%d\t%d\t%d\t%.3f\t%.3f\t%.3f\n",
			m.Threshold, m.TP, m.FP, m.FN, m.Precision, m.Recall, m.F1)
	}
	w.Flush()

	fmt.Println("\nprecision-first (dev-guide §6.3): prefer the largest threshold with precision 1.000")
}

func parseDisabled(s string) (map[string]bool, error) {
	if s == "" {
		return nil, nil
	}
	valid := map[string]bool{}
	for _, r := range fingerprint.RuleNames {
		valid[r] = true
	}
	out := map[string]bool{}
	for _, name := range strings.Split(s, ",") {
		name = strings.TrimSpace(name)
		if !valid[name] {
			return nil, fmt.Errorf("unknown rule %q (valid: %s)", name, strings.Join(fingerprint.RuleNames, ", "))
		}
		out[name] = true
	}
	return out, nil
}
