package textindex

import (
	"slices"
	"strings"

	"github.com/Back-to-code/f2f-match-utils/rules"
)

// noText marks an entry whose text has no words, which no rule can match.
const noText = ^uint32(0)

type owner struct {
	slot     uint32
	position uint32
}

// text is a distinct word sequence; every stored text with the same words shares it. A text in use
// has at least one owner: first, and the others in more, so the many texts with a single owner need
// no allocation for it.
type text struct {
	key   string
	words []uint32
	first owner
	more  []owner
}

// bucket lists the words that contain a gram. Retired words stay listed until more than half of the
// bucket is retired, so retiring a word never scans a long bucket.
type bucket struct {
	words   []uint32
	retired uint32
}

// corpus holds the distinct texts and the vocabulary that finds them. Postings are exact on every
// change, so a text id can be reused as soon as it is freed.
type corpus struct {
	textOf    map[string]uint32
	texts     []text
	freeTexts []uint32

	wordOf map[string]uint32
	// words[id] is "" once the word is retired: no text has it any more.
	words    []string
	postings [][]uint32
	// listed[id] counts the buckets that still list the word; a retired word's id is reused once no
	// bucket lists it.
	listed []uint32
	// lefts[id] and rights[id] are at most the length of the left and right word of every joined pair
	// the word is in, and 0 when it is also a word on its own; they are only ever lowered. A pair whose
	// left word is at least as long as a prefix starts with it only when that word does, so it adds
	// nothing to a prefix lookup; the same goes for right words and suffixes.
	lefts     []uint8
	rights    []uint8
	freeWords []uint32
	// The words of four letters or more, sorted forwards and backwards for prefix and suffix patterns,
	// and by gram4 of every four letters in them for infix patterns. Wildcard literals have at least
	// four letters, so shorter words never match one.
	forward  orderedWords
	backward orderedWords
	infixes  map[uint32]bucket

	normalizer rules.Normalizer
}

func newCorpus() corpus {
	return corpus{
		textOf:   map[string]uint32{},
		wordOf:   map[string]uint32{},
		backward: orderedWords{backward: true},
		infixes:  map[uint32]bucket{},
	}
}

var gramCode = func() [256]uint32 {
	var codes [256]uint32
	for ch := 'a'; ch <= 'z'; ch++ {
		codes[ch] = uint32(ch - 'a')
	}
	for ch := '0'; ch <= '9'; ch++ {
		codes[ch] = uint32(26 + ch - '0')
	}
	return codes
}()

func gram4(text string) uint32 {
	return ((gramCode[text[0]]*36+gramCode[text[1]])*36+gramCode[text[2]])*36 + gramCode[text[3]]
}

func (c *corpus) attach(raw string, by owner) uint32 {
	sequence := rules.SplitWords(raw)
	key := strings.Join(sequence, " ")
	if key == "" {
		return noText
	}
	textID, ok := c.textOf[key]
	if !ok {
		return c.addText(key, raw, sequence, by)
	}
	c.texts[textID].more = append(c.texts[textID].more, by)
	return textID
}

func (c *corpus) detach(textID uint32, by owner) {
	if textID == noText {
		return
	}
	stored := &c.texts[textID]
	last := len(stored.more) - 1
	switch {
	case last < 0:
		c.removeText(textID)
	case stored.first == by:
		stored.first = stored.more[last]
		stored.more = stored.more[:last]
	default:
		idx := slices.Index(stored.more, by)
		stored.more[idx] = stored.more[last]
		stored.more = stored.more[:last]
	}
}

func (c *corpus) addText(key, raw string, sequence []string, by owner) uint32 {
	var textID uint32
	if count := len(c.freeTexts); count > 0 {
		textID = c.freeTexts[count-1]
		c.freeTexts = c.freeTexts[:count-1]
	} else {
		textID = uint32(len(c.texts))
		c.texts = append(c.texts, text{})
	}
	words := c.normalizer.Words(raw)
	ids := make([]uint32, len(words))
	for idx, word := range words {
		ids[idx] = c.wordID(word)
		c.postings[ids[idx]] = append(c.postings[ids[idx]], textID)
		if idx < c.normalizer.RealWords {
			c.lefts[ids[idx]] = 0
			c.rights[ids[idx]] = 0
		}
	}
	for idx := 1; idx < len(sequence); idx++ {
		left, right := sequence[idx-1], sequence[idx]
		pair := ids[slices.Index(words, left+right)]
		c.lefts[pair] = min(c.lefts[pair], uint8(min(len(left), 255)))
		c.rights[pair] = min(c.rights[pair], uint8(min(len(right), 255)))
	}
	c.texts[textID] = text{key: key, words: ids, first: by}
	c.textOf[key] = textID
	return textID
}

func (c *corpus) removeText(textID uint32) {
	stored := c.texts[textID]
	for _, wordID := range stored.words {
		postings := c.postings[wordID]
		idx := slices.Index(postings, textID)
		postings[idx] = postings[len(postings)-1]
		c.postings[wordID] = postings[:len(postings)-1]
		if len(c.postings[wordID]) == 0 {
			c.retireWord(wordID)
		}
	}
	delete(c.textOf, stored.key)
	c.texts[textID] = text{}
	c.freeTexts = append(c.freeTexts, textID)
}

func (c *corpus) wordID(word string) uint32 {
	id, ok := c.wordOf[word]
	if ok {
		return id
	}
	// Normalizer words point into a buffer shared by the whole text; cloned so it can be collected.
	word = strings.Clone(word)
	if count := len(c.freeWords); count > 0 {
		id = c.freeWords[count-1]
		c.freeWords = c.freeWords[:count-1]
		c.words[id] = word
		c.lefts[id], c.rights[id] = 255, 255
	} else {
		id = uint32(len(c.words))
		c.words = append(c.words, word)
		c.postings = append(c.postings, nil)
		c.listed = append(c.listed, 0)
		c.lefts = append(c.lefts, 255)
		c.rights = append(c.rights, 255)
	}
	c.wordOf[word] = id
	if len(word) >= 4 {
		c.forward.insert(c.words, id)
		c.backward.insert(c.words, id)
	}
	c.forEachGram(word, func(gram uint32) {
		listing := c.infixes[gram]
		listing.words = append(listing.words, id)
		c.infixes[gram] = listing
		c.listed[id]++
	})
	return id
}

// retireWord forgets a word no text has any more. Its id is freed once no bucket lists it.
func (c *corpus) retireWord(id uint32) {
	word := c.words[id]
	if len(word) >= 4 {
		c.forward.remove(c.words, id)
		c.backward.remove(c.words, id)
	}
	delete(c.wordOf, word)
	c.words[id] = ""
	c.postings[id] = nil
	if c.listed[id] == 0 {
		c.freeWords = append(c.freeWords, id)
		return
	}
	c.forEachGram(word, func(gram uint32) {
		listing := c.infixes[gram]
		listing.retired++
		if listing.retired*2 > uint32(len(listing.words)) {
			c.compact(gram, listing)
			return
		}
		c.infixes[gram] = listing
	})
}

// compact drops the retired words from a bucket and frees the ids no bucket lists any more.
func (c *corpus) compact(gram uint32, listing bucket) {
	kept := listing.words[:0]
	for _, id := range listing.words {
		if c.words[id] != "" {
			kept = append(kept, id)
			continue
		}
		c.listed[id]--
		if c.listed[id] == 0 {
			c.freeWords = append(c.freeWords, id)
		}
	}
	if len(kept) == 0 {
		delete(c.infixes, gram)
		return
	}
	c.infixes[gram] = bucket{words: kept}
}

// forEachGram calls fn with each distinct gram in the word once.
func (c *corpus) forEachGram(word string, fn func(gram uint32)) {
	var buf [32]uint32
	seen := buf[:0]
	for start := 0; start+4 <= len(word); start++ {
		gram := gram4(word[start:])
		if slices.Contains(seen, gram) {
			continue
		}
		seen = append(seen, gram)
		fn(gram)
	}
}

func isWordText(text string) bool {
	for idx := range len(text) {
		ch := text[idx]
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') {
			return false
		}
	}
	return true
}

// wordsFor appends to buf the ids of the words the pattern matches. marks, when not nil, is an empty
// bitmap over word ids that it may use and empties again. Retired words never match: their text is
// "", and patterns are not.
func (c *corpus) wordsFor(pattern rules.Pattern, buf []uint32, marks bitmap) []uint32 {
	if pattern.Kind == rules.Exact {
		if id, ok := c.wordOf[pattern.Text]; ok {
			buf = append(buf, id)
		}
		return buf
	}
	text := pattern.Text
	// Only a syntax tree built without the parser holds a wildcard literal like this.
	if len(text) < 4 || !isWordText(text) {
		for id, word := range c.words {
			if word != "" && pattern.Matches(word) {
				buf = append(buf, uint32(id))
			}
		}
		return buf
	}
	// Words are a-z0-9, so the words that start (or end) with text sort before text with 0xff
	// after it (or before it).
	limit := uint8(min(len(text), 255))
	switch pattern.Kind {
	case rules.Prefix:
		return c.forward.appendRun(buf, c.words, text, text+"\xff", c.lefts, limit)
	case rules.Suffix:
		return c.backward.appendRun(buf, c.words, text, "\xff"+text, c.rights, limit)
	}
	var rarest, second []uint32
	for start := 0; start+4 <= len(text); start++ {
		listing := c.infixes[gram4(text[start:])].words
		switch {
		case start == 0 || len(listing) < len(rarest):
			rarest, second = listing, rarest
		case start == 1 || len(listing) < len(second):
			second = listing
		}
	}
	// A substring test reads the word, an id test does not: when the second rarest gram is not much
	// more common, only the words that have both grams are tested.
	if marks == nil || len(text) == 4 || len(second) > 16*len(rarest) {
		for _, id := range rarest {
			if pattern.Matches(c.words[id]) {
				buf = append(buf, id)
			}
		}
		return buf
	}
	for _, id := range rarest {
		marks.set(id)
	}
	for _, id := range second {
		if marks.has(id) && pattern.Matches(c.words[id]) {
			buf = append(buf, id)
		}
	}
	for _, id := range rarest {
		marks[id>>6] = 0
	}
	return buf
}

// estimate is the number of texts the pattern matches, counting a text once per word it matches.
func (c *corpus) estimate(pattern rules.Pattern) float64 {
	total := 0
	for _, id := range c.wordsFor(pattern, nil, nil) {
		total += len(c.postings[id])
	}
	return float64(total)
}
