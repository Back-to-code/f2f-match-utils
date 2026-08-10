// Package langcode holds the encoding the generated language table is written in. The
// generator and the runtime lookup both depend on it, so it lives in one place where they
// cannot drift apart. See the language package README for the reasoning.
package langcode

import (
	"fmt"
	"hash/fnv"
)

const (
	alphabetSize = 26
	firstLetter  = 'a'

	// Max is the highest packed code. Anything above it is not three letters.
	Max uint16 = alphabetSize * alphabetSize * alphabetSize
)

// FNV32a is the 32 bit FNV-1a hash of s, which is what the lookup names are matched on.
// The hasher does not escape, so this does not allocate.
func FNV32a(s string) uint32 {
	hasher := fnv.New32a()
	hasher.Write([]byte(s))
	return hasher.Sum32()
}

// PackBytes encodes a 3 letter ISO 639-3 code, lowercasing as it goes. Anything that is
// not a letter reports false: skipping one of three fixed bytes would leave a two letter
// code.
func PackBytes(code [3]byte) (uint16, bool) {
	packed := uint16(0)
	for _, letter := range code {
		lowered, ok := toLower(letter)
		if !ok {
			return 0, false
		}

		packed = packed*alphabetSize + uint16(lowered-firstLetter)
	}

	// Shifted up by one so the zero value can mean no language: "aaa" is a real code.
	return packed + 1, true
}

// Pack encodes an ISO 639-3 code written as text, lowercasing and skipping what is not a
// letter, so "NLD" and "n.l.d" both give nld. What survives has to be exactly three
// letters: padding or truncating "n/a" would quietly produce a different real language.
func Pack(code string) (uint16, error) {
	letters := [3]byte{}
	count := 0

	for i := range len(code) {
		lowered, ok := toLower(code[i])
		if !ok {
			continue
		}

		if count == len(letters) {
			return 0, fmt.Errorf("%q holds more than 3 letters", code)
		}

		letters[count] = lowered
		count++
	}

	if count != len(letters) {
		return 0, fmt.Errorf("%q holds %d letters, a code has 3", code, count)
	}

	packed, _ := PackBytes(letters)

	return packed, nil
}

func toLower(letter byte) (byte, bool) {
	if letter >= 'A' && letter <= 'Z' {
		letter += firstLetter - 'A'
	}

	if letter < firstLetter || letter > 'z' {
		return 0, false
	}

	return letter, true
}

// Unpack is the inverse of PackBytes, and reports false for a value that spells no code.
func Unpack(packed uint16) ([3]byte, bool) {
	if packed == 0 || packed > Max {
		return [3]byte{}, false
	}

	packed--

	return [3]byte{
		byte(packed/(alphabetSize*alphabetSize)) + firstLetter,
		byte(packed/alphabetSize%alphabetSize) + firstLetter,
		byte(packed%alphabetSize) + firstLetter,
	}, true
}
