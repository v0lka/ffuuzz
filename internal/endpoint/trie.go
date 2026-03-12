package endpoint

import "strings"

// collapseRatio is the minimum ratio of unique literal children to total
// observations for a statistical collapse to trigger.
const collapseRatio = 0.3

// collapseMinChildren is the minimum number of unique literal children
// required for a statistical collapse.
const collapseMinChildren = 3

// collapseMinWithPlaceholder is the minimum number of unique literal children
// required when a {_} sibling already exists (heuristic-assisted collapse).
const collapseMinWithPlaceholder = 2

// trieNode is a node in a segment trie that tracks cardinality per position.
type trieNode struct {
	children         map[string]*trieNode
	observationCount int // number of distinct recordings through this node
}

func newTrieNode() *trieNode {
	return &trieNode{children: make(map[string]*trieNode)}
}

// observe walks or creates the trie path for the given segments and increments
// observation counts. It returns the list of trie nodes traversed (excluding
// the root).
func (n *trieNode) observe(segments []string) {
	cur := n
	for _, seg := range segments {
		cur.observationCount++
		child, ok := cur.children[seg]
		if !ok {
			child = newTrieNode()
			cur.children[seg] = child
		}
		cur = child
	}
	// Count the leaf node itself.
	cur.observationCount++
}

// CollapseResult describes a detected collapse: the position in the path
// (0-based segment index) and the affected parent prefix segments.
type CollapseResult struct {
	// Position is the 0-based segment index where the collapse should happen
	// (i.e., the children at this depth should be merged into {_}).
	Position int
	// ParentSegments is the path prefix up to (but not including) Position.
	ParentSegments []string
}

// checkCollapse walks the trie and returns the first position that should be
// collapsed, if any. It inspects the trie globally, not just the path of the
// last observation.
func (n *trieNode) checkCollapse() (CollapseResult, bool) {
	return n.checkCollapseAt(nil)
}

func (n *trieNode) checkCollapseAt(prefix []string) (CollapseResult, bool) {
	if len(n.children) == 0 {
		return CollapseResult{}, false
	}

	literalCount := 0
	hasPlaceholder := false
	for key := range n.children {
		if key == Placeholder {
			hasPlaceholder = true
		} else {
			literalCount++
		}
	}

	if shouldCollapse(literalCount, n.observationCount, hasPlaceholder) {
		return CollapseResult{
			Position:       len(prefix),
			ParentSegments: prefix,
		}, true
	}

	// Recurse into children.
	for key, child := range n.children {
		if key == Placeholder {
			continue
		}
		if result, ok := child.checkCollapseAt(append(prefix, key)); ok {
			return result, true
		}
	}

	return CollapseResult{}, false
}

// shouldCollapse implements the two collapse conditions.
func shouldCollapse(literalCount, observationCount int, hasPlaceholder bool) bool {
	// Condition A: pure statistical collapse.
	if literalCount >= collapseMinChildren && observationCount > 0 {
		ratio := float64(literalCount) / float64(observationCount)
		if ratio > collapseRatio {
			return true
		}
	}

	// Condition B: heuristic-assisted collapse.
	// If a {_} child already exists (heuristic already detected parameters at
	// this level), a lower threshold applies.
	if hasPlaceholder && literalCount >= collapseMinWithPlaceholder {
		return true
	}

	return false
}

// collapse merges all literal children at the given position into {_}.
// It returns the list of literal keys that were collapsed.
func (n *trieNode) collapse(parentSegments []string) []string {
	// Walk to the parent node of the collapse position.
	cur := n
	for _, seg := range parentSegments {
		child, ok := cur.children[seg]
		if !ok {
			return nil
		}
		cur = child
	}

	// Collect literal children to merge.
	var collapsed []string
	placeholder, hasExisting := cur.children[Placeholder]
	if !hasExisting {
		placeholder = newTrieNode()
	}

	for key, child := range cur.children {
		if key == Placeholder {
			continue
		}
		collapsed = append(collapsed, key)

		// Merge observation count and children into the placeholder node.
		placeholder.observationCount += child.observationCount
		for subKey, subChild := range child.children {
			if existing, ok := placeholder.children[subKey]; ok {
				existing.observationCount += subChild.observationCount
				mergeChildren(existing, subChild)
			} else {
				placeholder.children[subKey] = subChild
			}
		}

		delete(cur.children, key)
	}

	if len(collapsed) > 0 {
		cur.children[Placeholder] = placeholder
	}

	return collapsed
}

// mergeChildren recursively merges src's children into dst.
func mergeChildren(dst, src *trieNode) {
	for key, srcChild := range src.children {
		if dstChild, ok := dst.children[key]; ok {
			dstChild.observationCount += srcChild.observationCount
			mergeChildren(dstChild, srcChild)
		} else {
			dst.children[key] = srcChild
		}
	}
}

// collapsedPatterns returns all recording target_path patterns that are
// affected by a collapse at the given parent. The patterns have the collapsed
// segment replaced with {_}.
func collapsedPatterns(parentSegments, collapsedKeys []string) (sourcePatterns []string, targetPattern string) {
	// Build the target pattern: parentSegments + {_} + ...
	// We only know the parent prefix; the full patterns are parent/{collapsed_key}/...
	// For the merge, source patterns are parent/{literal} and target is parent/{_}.
	// Since recordings may have deeper paths, we need prefix-based matching in the DB.
	// Here we return the parent prefix for the caller to use in a LIKE query.

	parentPath := "/" + strings.Join(parentSegments, "/")
	if len(parentSegments) > 0 {
		parentPath += "/"
	}

	for _, key := range collapsedKeys {
		sourcePatterns = append(sourcePatterns, parentPath+key)
	}

	targetPattern = parentPath + Placeholder
	return sourcePatterns, targetPattern
}
