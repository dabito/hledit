package main

import (
	"fmt"
	"regexp"
)

var anchorRE = regexp.MustCompile(`^\s*(\d+)\s*#\s*([ABCDEFGHJKLMNPQRSTUVWXYZ23456789]{3}|[ZPMQVRWSNKTXJBYH]{2})(?:\b|[^A-Za-z0-9])`)

// parseAnchor parses a single-line anchor string like "5#ABC" or legacy "5#WS".
func parseAnchor(s string) (Anchor, error) {
	matches := anchorRE.FindStringSubmatch(s)
	if len(matches) != 3 {
		return Anchor{}, fmt.Errorf("invalid anchor %q: expected LN#HASH or legacy LN#HH (e.g. \"5#ABC\" or \"5#WS\")", s)
	}

	var lineNum int
	_, err := fmt.Sscanf(matches[1], "%d", &lineNum)
	if err != nil {
		return Anchor{}, err
	}

	if lineNum < 1 {
		return Anchor{}, fmt.Errorf("anchor line number must be >= 1, got %d in %q", lineNum, s)
	}

	return Anchor{Line: lineNum, Hash: matches[2]}, nil
}

// validateAnchor checks if the anchor matches the current content of the line.
func validateAnchor(lines []string, a Anchor) error {
	if a.Line < 1 || a.Line > len(lines) {
		return fmt.Errorf("anchor line %d out of range (file has %d lines)", a.Line, len(lines))
	}

	if anchorMatches(a, lines[a.Line-1]) {
		return nil
	}

	actualHash := computeLineHash(a.Line, lines[a.Line-1])
	return fmt.Errorf("anchor %d#%s: expected hash %s, got %s", a.Line, a.Hash, a.Hash, actualHash)
}

func anchorMatches(a Anchor, line string) bool {
	switch len(a.Hash) {
	case 2:
		return computeLegacyLineHash(a.Line, line) == a.Hash
	case 3:
		return computeLineHash(a.Line, line) == a.Hash
	default:
		return false
	}
}

// validateAnchors iterates through a set of anchors and returns remaps for any stale ones.
// Out-of-range anchors are treated as stale with an empty Current tag.
func validateAnchors(lines []string, anchors []Anchor) (remaps []Remap, firstBad int) {
	firstBad = -1
	for i, a := range anchors {
		requestedTag := intToStr(a.Line) + "#" + a.Hash

		var currentTag string
		if a.Line >= 1 && a.Line <= len(lines) {
			currentTag = formatTag(a.Line, lines[a.Line-1])
		}

		current := a.Line >= 1 && a.Line <= len(lines) && anchorMatches(a, lines[a.Line-1])
		if !current {
			if firstBad == -1 {
				firstBad = i
			}
			remaps = append(remaps, Remap{
				Requested: requestedTag,
				Current:   currentTag,
			})
		}
	}
	return remaps, firstBad
}
