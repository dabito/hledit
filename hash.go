package main

import (
	"hash/fnv"
	"strings"
	"unicode"
)

const legacyAnchorAlphabet = "ZPMQVRWSNKTXJBYH"

// anchorAlphabet is uppercase base32 without 0/O or 1/I. L is kept.
const anchorAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// computeLineHash computes a 3-character base32 hash for a given line number and line content.
func computeLineHash(lineNum int, line string) string {
	return encodeLineHash(lineNum, line, anchorAlphabet, 3)
}

// computeLegacyLineHash computes the pre-1.3.0 2-character base16 hash for compatibility.
func computeLegacyLineHash(lineNum int, line string) string {
	return encodeLineHash(lineNum, line, legacyAnchorAlphabet, 2)
}

func encodeLineHash(lineNum int, line string, alphabet string, length int) string {
	line = normalizedHashLine(line)

	sum := lineHashSum(lineNum, line)
	buf := make([]byte, length)
	base := uint32(len(alphabet))
	for i := length - 1; i >= 0; i-- {
		buf[i] = alphabet[sum%base]
		sum /= base
	}
	return string(buf)
}

func normalizedHashLine(line string) string {
	line = strings.TrimRight(line, "\r")
	return strings.TrimRightFunc(line, unicode.IsSpace)
}

func lineHashSum(lineNum int, line string) uint32 {
	h := fnv.New32a()

	// Determine if line is "significant": contains at least one unicode.IsLetter OR unicode.IsDigit rune.
	isSignificant := false
	for _, r := range line {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			isSignificant = true
			break
		}
	}

	// If NOT significant, mix lineNum into the FNV-1a state BEFORE the content.
	if !isSignificant {
		n := lineNum
		for n > 0 {
			h.Write([]byte{byte(n & 0xff)})
			n >>= 8
		}
	}

	h.Write([]byte(line))
	return h.Sum32()
}

// formatTag returns intToStr(lineNum) + "#" + computeLineHash(lineNum, line).
func formatTag(lineNum int, line string) string {
	return intToStr(lineNum) + "#" + computeLineHash(lineNum, line)
}

// intToStr converts an integer to a decimal string WITHOUT fmt (avoid allocations).
// It handles 0 and negatives using a fixed [20]byte buffer, building digits right-to-left.
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}

	// Using a 22-byte buffer to safely handle sign and digits for 64-bit int.
	var buf [22]byte
	i := 22

	neg := false
	var un uint64
	if n < 0 {
		neg = true
		// Using unsigned conversion to handle MinInt correctly.
		un = uint64(-n)
	} else {
		un = uint64(n)
	}

	for un > 0 {
		i--
		buf[i] = byte('0' + (un % 10))
		un /= 10
	}

	if neg {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}
