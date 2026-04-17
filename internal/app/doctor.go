package app

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/GetModus/modus-memory/internal/index"
	"github.com/GetModus/modus-memory/internal/markdown"
)

func runDoctor(vaultDir string) {
	fmt.Printf("%s doctor %s\n", commandName(), Version)
	fmt.Printf("Vault: %s\n\n", vaultDir)

	idx, err := index.Build(vaultDir, "")
	if err != nil {
		fmt.Printf("FAIL: cannot build index: %v\n", err)
		os.Exit(1)
	}

	totalFacts, activeFacts := idx.FactCount()
	subjects, tags, entities := idx.CrossRefStats()

	fmt.Printf("Documents: %d\n", idx.DocCount())
	fmt.Printf("Facts: %d total, %d active, %d archived\n", totalFacts, activeFacts, totalFacts-activeFacts)
	fmt.Printf("Cross-refs: %d subjects, %d tags, %d entities\n\n", subjects, tags, entities)

	docs, err := markdown.ScanDir(vaultDir)
	if err != nil {
		fmt.Printf("FAIL: cannot scan vault: %v\n", err)
		os.Exit(1)
	}

	var findings []finding

	missingSubject := 0
	missingPredicate := 0
	for _, doc := range docs {
		if !strings.Contains(doc.Path, "memory/facts") {
			continue
		}
		if doc.Get("subject") == "" {
			missingSubject++
		}
		if doc.Get("predicate") == "" {
			missingPredicate++
		}
	}
	if missingSubject > 0 {
		findings = append(findings, finding{"WARN", fmt.Sprintf("%d facts missing 'subject' field", missingSubject)})
	}
	if missingPredicate > 0 {
		findings = append(findings, finding{"WARN", fmt.Sprintf("%d facts missing 'predicate' field", missingPredicate)})
	}

	type factKey struct{ subject, predicate string }
	factCounts := make(map[factKey]int)
	for _, doc := range docs {
		if !strings.Contains(doc.Path, "memory/facts") {
			continue
		}
		s := doc.Get("subject")
		p := doc.Get("predicate")
		if s != "" && p != "" {
			factCounts[factKey{s, p}]++
		}
	}
	dupes := 0
	for _, count := range factCounts {
		if count > 1 {
			dupes++
		}
	}
	if dupes > 0 {
		findings = append(findings, finding{"WARN", fmt.Sprintf("%d duplicate subject+predicate pairs", dupes)})
	}

	emptyDocs := 0
	for _, doc := range docs {
		if strings.TrimSpace(doc.Body) == "" {
			emptyDocs++
		}
	}
	if emptyDocs > 0 {
		findings = append(findings, finding{"INFO", fmt.Sprintf("%d documents with empty body", emptyDocs)})
	}

	noFrontmatter := 0
	for _, doc := range docs {
		if len(doc.Frontmatter) == 0 {
			noFrontmatter++
		}
	}
	if noFrontmatter > 0 {
		findings = append(findings, finding{"INFO", fmt.Sprintf("%d documents without frontmatter", noFrontmatter)})
	}

	type factEntry struct {
		value string
		path  string
	}
	factValues := make(map[factKey][]factEntry)
	for _, doc := range docs {
		if !strings.Contains(doc.Path, "memory/facts") {
			continue
		}
		s := doc.Get("subject")
		p := doc.Get("predicate")
		if s == "" || p == "" {
			continue
		}
		v := strings.TrimSpace(doc.Body)
		if v == "" {
			v = doc.Get("value")
		}
		if len(v) > 100 {
			v = v[:100]
		}
		key := factKey{strings.ToLower(s), strings.ToLower(p)}
		rel := doc.Path
		if idx := strings.Index(rel, "memory/facts"); idx >= 0 {
			rel = rel[idx:]
		}
		factValues[key] = append(factValues[key], factEntry{v, rel})
	}
	contradictions := 0
	var contradictionDetails []string
	for key, entries := range factValues {
		if len(entries) < 2 {
			continue
		}
		seen := make(map[string]bool)
		for _, e := range entries {
			seen[e.value] = true
		}
		if len(seen) > 1 {
			contradictions++
			if len(contradictionDetails) < 5 {
				contradictionDetails = append(contradictionDetails, fmt.Sprintf("  %s / %s (%d conflicting values)", key.subject, key.predicate, len(seen)))
			}
		}
	}
	if contradictions > 0 {
		findings = append(findings, finding{"WARN", fmt.Sprintf("%d potential contradictions (same subject+predicate, different values)", contradictions)})
	}

	expectedDirs := []string{"memory/facts", "brain", "atlas"}
	for _, dir := range expectedDirs {
		full := fmt.Sprintf("%s/%s", vaultDir, dir)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			findings = append(findings, finding{"INFO", fmt.Sprintf("directory %s/ does not exist (optional)", dir)})
		}
	}

	dirCounts := make(map[string]int)
	for _, doc := range docs {
		parts := strings.SplitN(doc.Path, string(os.PathSeparator), 3)
		if len(parts) >= 2 {
			rel := doc.Path
			if idx := strings.LastIndex(rel, vaultDir); idx >= 0 {
				rel = rel[idx+len(vaultDir)+1:]
			}
			topParts := strings.SplitN(rel, string(os.PathSeparator), 3)
			if len(topParts) >= 1 {
				dirCounts[topParts[0]]++
			}
		}
	}

	fmt.Println("─── Diagnostics ───")
	if len(findings) == 0 {
		fmt.Println("No issues found.")
	} else {
		sort.Slice(findings, func(i, j int) bool {
			return severityRank(findings[i].level) > severityRank(findings[j].level)
		})
		for _, f := range findings {
			fmt.Printf("[%s] %s\n", f.level, f.message)
		}
	}

	if len(contradictionDetails) > 0 {
		fmt.Println("\n─── Contradictions (first 5) ───")
		for _, d := range contradictionDetails {
			fmt.Println(d)
		}
		if contradictions > 5 {
			fmt.Printf("  ... and %d more\n", contradictions-5)
		}
	}

	if len(dirCounts) > 0 {
		fmt.Println("\n─── Distribution ───")
		type dirStat struct {
			name  string
			count int
		}
		var stats []dirStat
		for name, count := range dirCounts {
			stats = append(stats, dirStat{name, count})
		}
		sort.Slice(stats, func(i, j int) bool { return stats[i].count > stats[j].count })
		for _, s := range stats {
			fmt.Printf("  %-20s %d docs\n", s.name+"/", s.count)
		}
	}

	fmt.Println()
	if len(findings) == 0 {
		fmt.Println("Vault is healthy.")
	} else {
		warns := 0
		for _, f := range findings {
			if f.level == "WARN" {
				warns++
			}
		}
		if warns > 0 {
			fmt.Printf("%d warnings, %d info. Run after cleanup to verify.\n", warns, len(findings)-warns)
		} else {
			fmt.Printf("%d info items. Vault is healthy.\n", len(findings))
		}
	}
}

type finding struct {
	level   string
	message string
}

func severityRank(level string) int {
	switch level {
	case "FAIL":
		return 3
	case "WARN":
		return 2
	case "INFO":
		return 1
	default:
		return 0
	}
}
