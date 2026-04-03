package index

import (
	"fmt"
	"sort"
	"strings"
)

// CrossRef is an in-memory cross-index that connects documents sharing subjects,
// entities, or tags. Built at index time alongside BM25. No new storage — just
// adjacency maps derived from existing frontmatter.
//
// This is Option A from the knowledge graph discussion: connected search results
// without a full graph engine. A query for "Gemma 4" returns the fact, the Atlas
// entity, the beliefs, the articles that mention it, and the learnings — one search.

// DocRef is a lightweight reference to a connected document.
type DocRef struct {
	Path    string
	Title   string
	Kind    string // "fact", "belief", "entity", "article", "learning", "mission", "role"
	Subject string
	Rank    float64 // connection strength (number of shared terms)
}

// crossIndex maps normalized subjects/entities to document references.
type crossIndex struct {
	bySubject map[string][]DocRef // lowercase subject → connected docs
	byTag     map[string][]DocRef // tag → connected docs
	byEntity  map[string][]DocRef // entity name → connected docs
}

func newCrossIndex() *crossIndex {
	return &crossIndex{
		bySubject: make(map[string][]DocRef),
		byTag:     make(map[string][]DocRef),
		byEntity:  make(map[string][]DocRef),
	}
}

// build populates the cross-index from all indexed documents.
func (ci *crossIndex) build(docs []document) {
	for _, doc := range docs {
		kind := inferDocKind(doc.Path, doc.Kind)
		ref := DocRef{
			Path:    doc.Path,
			Title:   doc.Title,
			Kind:    kind,
			Subject: doc.Subject,
		}

		// Index by subject
		if doc.Subject != "" {
			key := strings.ToLower(strings.TrimSpace(doc.Subject))
			ci.bySubject[key] = append(ci.bySubject[key], ref)
		}

		// Index by tags
		for _, tag := range strings.Fields(doc.Tags) {
			tag = strings.ToLower(strings.TrimSpace(tag))
			if tag != "" {
				ci.byTag[tag] = append(ci.byTag[tag], ref)
			}
		}

		// Index by entity mentions in title and subject
		// Entities are subjects of atlas docs, but also appear in other docs' text.
		// For now, cross-reference by exact subject match across all doc types.
		if kind == "entity" && doc.Title != "" {
			key := strings.ToLower(strings.TrimSpace(doc.Title))
			ci.byEntity[key] = append(ci.byEntity[key], ref)
		}
		if kind == "entity" && doc.Subject != "" {
			key := strings.ToLower(strings.TrimSpace(doc.Subject))
			ci.byEntity[key] = append(ci.byEntity[key], ref)
		}

		// Scan body for references to known entities (built in second pass)
	}

	// Second pass: scan all docs for mentions of entity names in title/body.
	// This connects articles to entities they discuss.
	entityNames := make(map[string]bool, len(ci.byEntity))
	for name := range ci.byEntity {
		if len(name) >= 3 { // skip tiny entity names
			entityNames[name] = true
		}
	}

	for _, doc := range docs {
		kind := inferDocKind(doc.Path, doc.Kind)
		if kind == "entity" {
			continue // entities already indexed
		}

		ref := DocRef{
			Path:    doc.Path,
			Title:   doc.Title,
			Kind:    kind,
			Subject: doc.Subject,
		}

		// Check if title or subject mentions a known entity
		titleLower := strings.ToLower(doc.Title)
		subjectLower := strings.ToLower(doc.Subject)

		for entityName := range entityNames {
			if strings.Contains(titleLower, entityName) || strings.Contains(subjectLower, entityName) {
				ci.byEntity[entityName] = append(ci.byEntity[entityName], ref)
			}
		}
	}
}

// ForSubject returns all documents connected to a subject (case-insensitive).
func (ci *crossIndex) ForSubject(subject string, limit int) []DocRef {
	if limit <= 0 {
		limit = 20
	}
	key := strings.ToLower(strings.TrimSpace(subject))
	refs := ci.bySubject[key]
	if len(refs) > limit {
		refs = refs[:limit]
	}
	return refs
}

// ForTag returns all documents sharing a tag.
func (ci *crossIndex) ForTag(tag string, limit int) []DocRef {
	if limit <= 0 {
		limit = 20
	}
	key := strings.ToLower(strings.TrimSpace(tag))
	refs := ci.byTag[key]
	if len(refs) > limit {
		refs = refs[:limit]
	}
	return refs
}

// ForEntity returns all documents connected to an entity.
func (ci *crossIndex) ForEntity(entity string, limit int) []DocRef {
	if limit <= 0 {
		limit = 20
	}
	key := strings.ToLower(strings.TrimSpace(entity))
	refs := ci.byEntity[key]
	if len(refs) > limit {
		refs = refs[:limit]
	}
	return refs
}

// Connected returns the full neighborhood of a query — union of subject, tag,
// and entity matches, deduplicated by path, sorted by connection count.
func (ci *crossIndex) Connected(query string, limit int) []DocRef {
	if limit <= 0 {
		limit = 20
	}

	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return nil
	}

	// Score each document by how many query terms connect to it
	scores := make(map[string]*DocRef)  // path → ref
	counts := make(map[string]float64)  // path → connection strength

	addRefs := func(refs []DocRef, weight float64) {
		for _, r := range refs {
			counts[r.Path] += weight
			if _, ok := scores[r.Path]; !ok {
				copy := r
				scores[r.Path] = &copy
			}
		}
	}

	for _, term := range terms {
		addRefs(ci.bySubject[term], 3.0) // subject match is strongest
		addRefs(ci.byEntity[term], 2.0)  // entity match is strong
		addRefs(ci.byTag[term], 1.0)     // tag match is weaker
	}

	// Also try the full query as a single key
	fullQuery := strings.Join(terms, " ")
	addRefs(ci.bySubject[fullQuery], 3.0)
	addRefs(ci.byEntity[fullQuery], 2.0)

	// Sort by connection strength
	type ranked struct {
		ref   *DocRef
		score float64
	}
	var results []ranked
	for path, ref := range scores {
		ref.Rank = counts[path]
		results = append(results, ranked{ref, counts[path]})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	out := make([]DocRef, len(results))
	for i, r := range results {
		out[i] = *r.ref
	}
	return out
}

// Stats returns cross-index statistics.
func (ci *crossIndex) Stats() (subjects, tags, entities int) {
	return len(ci.bySubject), len(ci.byTag), len(ci.byEntity)
}

// inferDocKind determines the document type from its vault path and metadata.
func inferDocKind(path, metaKind string) string {
	if metaKind != "" {
		return metaKind
	}
	switch {
	case strings.HasPrefix(path, "memory/facts/"):
		return "fact"
	case strings.HasPrefix(path, "atlas/beliefs/"):
		return "belief"
	case strings.HasPrefix(path, "atlas/entities/") || strings.HasPrefix(path, "atlas/"):
		return "entity"
	case strings.HasPrefix(path, "brain/learnings/"):
		return "learning"
	case strings.HasPrefix(path, "brain/"):
		return "article"
	case strings.HasPrefix(path, "missions/"):
		return "mission"
	case strings.HasPrefix(path, "roles/"):
		return "role"
	case strings.HasPrefix(path, "experience/"):
		return "experience"
	default:
		return "document"
	}
}

// FormatConnected returns a human-readable string of connected documents.
func FormatConnected(refs []DocRef) string {
	if len(refs) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Connected (%d)\n\n", len(refs)))

	// Group by kind
	byKind := make(map[string][]DocRef)
	for _, r := range refs {
		byKind[r.Kind] = append(byKind[r.Kind], r)
	}

	kindOrder := []string{"fact", "belief", "entity", "learning", "article", "mission", "experience", "role", "document"}
	for _, kind := range kindOrder {
		group := byKind[kind]
		if len(group) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("**%ss** (%d):\n", kind, len(group)))
		for _, r := range group {
			title := r.Title
			if title == "" {
				title = r.Path
			}
			sb.WriteString(fmt.Sprintf("- %s `%s`\n", title, r.Path))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
