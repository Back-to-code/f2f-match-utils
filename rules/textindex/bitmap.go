package textindex

import "math/bits"

type bitmap []uint64

func newBitmap(size int) bitmap { return make(bitmap, (size+63)/64) }

func (b bitmap) set(idx uint32) { b[idx>>6] |= 1 << (idx & 63) }

func (b bitmap) has(idx uint32) bool { return b[idx>>6]&(1<<(idx&63)) != 0 }

func (b bitmap) or(other bitmap) {
	for idx := range b {
		b[idx] |= other[idx]
	}
}

// orClear moves other into b, leaving other empty.
func (b bitmap) orClear(other bitmap) {
	for idx, word := range other {
		if word != 0 {
			b[idx] |= word
			other[idx] = 0
		}
	}
}

// andClear intersects b with other, leaves other empty and reports whether anything is left in b.
func (b bitmap) andClear(other bitmap) bool {
	var any uint64
	for idx := range b {
		b[idx] &= other[idx]
		other[idx] = 0
		any |= b[idx]
	}
	return any != 0
}

// andNotClear removes other from b, leaves other empty and reports whether anything is left in b.
func (b bitmap) andNotClear(other bitmap) bool {
	var any uint64
	for idx := range b {
		b[idx] &^= other[idx]
		other[idx] = 0
		any |= b[idx]
	}
	return any != 0
}

// forEach calls fn for every set bit in ascending order.
func (b bitmap) forEach(fn func(idx uint32)) {
	for wordIdx, word := range b {
		for word != 0 {
			fn(uint32(wordIdx*64 + bits.TrailingZeros64(word)))
			word &= word - 1
		}
	}
}
