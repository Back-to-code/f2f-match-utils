package textindex

import "sort"

// chunkSize bounds a chunk of orderedWords: inserting or removing a word moves at most this many ids.
const chunkSize = 128

// orderedWords keeps word ids sorted by their word, read forwards or backwards, so every word with a
// given start (or end) is one run. The ids sit in chunks, so a change moves one chunk, not all of them.
type orderedWords struct {
	chunks [][]uint32
	// fences[i] is the key8 of the last word of chunks[i]. Finding the chunk then mostly compares
	// these instead of reading words scattered over the heap.
	fences   []uint64
	backward bool
}

// key8 packs the first eight bytes of text (its last eight, read backwards, for a backward order) so
// that comparing keys orders texts like comparing them in full does, unless the keys are equal.
// Words hold no zero bytes, so the padding sorts a shorter text first, as the full comparison does.
func (o *orderedWords) key8(text string) uint64 {
	var key uint64
	for idx := range 8 {
		key <<= 8
		if idx < len(text) {
			if o.backward {
				key |= uint64(text[len(text)-1-idx])
			} else {
				key |= uint64(text[idx])
			}
		}
	}
	return key
}

// compareBackward compares the strings read from their last byte, so suffixes sort together.
func compareBackward(left, right string) int {
	for idx := 1; idx <= len(left) && idx <= len(right); idx++ {
		a, b := left[len(left)-idx], right[len(right)-idx]
		if a != b {
			if a < b {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(left) < len(right):
		return -1
	case len(left) > len(right):
		return 1
	}
	return 0
}

func compareForward(left, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	}
	return 0
}

func (o *orderedWords) compare(left, right string) int {
	if o.backward {
		return compareBackward(left, right)
	}
	return compareForward(left, right)
}

// search returns the position of the first word that sorts at or after key.
func (o *orderedWords) search(words []string, key string) (int, int) {
	keyed := o.key8(key)
	chunk := sort.Search(len(o.chunks), func(idx int) bool {
		if fence := o.fences[idx]; fence != keyed {
			return fence > keyed
		}
		ids := o.chunks[idx]
		return o.compare(words[ids[len(ids)-1]], key) >= 0
	})
	if chunk == len(o.chunks) {
		return chunk, 0
	}
	ids := o.chunks[chunk]
	return chunk, sort.Search(len(ids), func(idx int) bool { return o.compare(words[ids[idx]], key) >= 0 })
}

func (o *orderedWords) refence(words []string, chunk int) {
	ids := o.chunks[chunk]
	o.fences[chunk] = o.key8(words[ids[len(ids)-1]])
}

func (o *orderedWords) insert(words []string, id uint32) {
	if len(o.chunks) == 0 {
		o.chunks = [][]uint32{append(make([]uint32, 0, chunkSize), id)}
		o.fences = []uint64{o.key8(words[id])}
		return
	}
	chunk, idx := o.search(words, words[id])
	if chunk == len(o.chunks) {
		chunk--
		idx = len(o.chunks[chunk])
	}
	if len(o.chunks[chunk]) == chunkSize {
		ids := o.chunks[chunk]
		upper := append(make([]uint32, 0, chunkSize), ids[chunkSize/2:]...)
		o.chunks[chunk] = ids[:chunkSize/2]
		o.chunks = append(o.chunks, nil)
		copy(o.chunks[chunk+2:], o.chunks[chunk+1:])
		o.chunks[chunk+1] = upper
		o.fences = append(o.fences, 0)
		copy(o.fences[chunk+2:], o.fences[chunk+1:])
		o.fences[chunk+1] = o.fences[chunk]
		o.refence(words, chunk)
		if idx > chunkSize/2 {
			chunk, idx = chunk+1, idx-chunkSize/2
		}
	}
	ids := append(o.chunks[chunk], 0)
	copy(ids[idx+1:], ids[idx:])
	ids[idx] = id
	o.chunks[chunk] = ids
	if idx == len(ids)-1 {
		o.refence(words, chunk)
	}
}

// remove takes out the id of a word that is in the list; words[id] must still hold the word. A chunk
// that gets small is merged into a neighbour, so churn does not leave many near-empty chunks.
func (o *orderedWords) remove(words []string, id uint32) {
	chunk, idx := o.search(words, words[id])
	ids := o.chunks[chunk]
	ids = append(ids[:idx], ids[idx+1:]...)
	o.chunks[chunk] = ids
	if len(ids) > 0 && idx == len(ids) {
		o.refence(words, chunk)
	}
	if len(ids) >= chunkSize/4 {
		return
	}
	switch {
	case chunk+1 < len(o.chunks) && len(ids)+len(o.chunks[chunk+1]) <= chunkSize*3/4:
		o.chunks[chunk] = append(ids, o.chunks[chunk+1]...)
		o.fences[chunk] = o.fences[chunk+1]
		o.drop(chunk + 1)
	case chunk > 0 && len(ids)+len(o.chunks[chunk-1]) <= chunkSize*3/4:
		o.chunks[chunk-1] = append(o.chunks[chunk-1], ids...)
		if len(ids) > 0 {
			o.fences[chunk-1] = o.fences[chunk]
		}
		o.drop(chunk)
	case len(ids) == 0:
		o.drop(chunk)
	}
}

func (o *orderedWords) drop(chunk int) {
	o.chunks = append(o.chunks[:chunk], o.chunks[chunk+1:]...)
	o.fences = append(o.fences[:chunk], o.fences[chunk+1:]...)
}

// appendRun appends the ids of the words that sort at or after from and before to, leaving out those
// whose split is limit or more.
func (o *orderedWords) appendRun(buf []uint32, words []string, from, to string, splits []uint8, limit uint8) []uint32 {
	chunk, idx := o.search(words, from)
	endChunk, endIdx := o.search(words, to)
	for ; chunk <= endChunk && chunk < len(o.chunks); chunk, idx = chunk+1, 0 {
		ids := o.chunks[chunk][idx:]
		if chunk == endChunk {
			ids = o.chunks[chunk][idx:endIdx]
		}
		for _, id := range ids {
			if splits[id] < limit {
				buf = append(buf, id)
			}
		}
	}
	return buf
}
