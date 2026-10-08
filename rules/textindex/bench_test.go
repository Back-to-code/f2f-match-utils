package textindex

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/Back-to-code/f2f-match-utils/rules"
	"github.com/Back-to-code/f2f-match-utils/rules/internal/synth"
)

// Run: go test -run '^$' -bench . -benchmem ./rules/textindex (the 1M case builds for a while).

type benchCorpus struct {
	ix    *Index[uint32]
	build time.Duration
	heap  uint64
}

var benchCorpora = map[int]*benchCorpus{}

func heapInUse() uint64 {
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

// benchIndex stores size texts under IDs of one to five texts each.
func benchIndex(b *testing.B, size int) *benchCorpus {
	b.Helper()
	if corpus, ok := benchCorpora[size]; ok {
		return corpus
	}
	gen := synth.New(7, size/20)
	all := make([]string, size)
	for idx := range all {
		all[idx] = gen.Text()
	}
	before := heapInUse()
	start := time.Now()
	ix := New[uint32]()
	for id, offset := uint32(0), 0; offset < size; id++ {
		count := min(1+int(id%5), size-offset)
		ix.Set(id, all[offset:offset+count]...)
		offset += count
	}
	corpus := &benchCorpus{ix: ix, build: time.Since(start)}
	all = nil
	corpus.heap = heapInUse() - before
	benchCorpora[size] = corpus
	return corpus
}

func benchSets(count, includes int) []*rules.Compiled {
	gen := synth.New(8, 2000)
	out := make([]*rules.Compiled, count)
	for idx := range out {
		out[idx] = rules.MustCompile(gen.Set(includes, 3))
	}
	return out
}

func BenchmarkMatch(b *testing.B) {
	sets := benchSets(50, 20)
	for _, size := range []int{100_000, 1_000_000} {
		b.Run(fmt.Sprintf("texts=%d", size), func(b *testing.B) {
			corpus := benchIndex(b, size)
			hits, idx := 0, 0
			b.ResetTimer()
			for b.Loop() {
				hits += len(corpus.ix.Match(sets[idx%len(sets)]))
				idx++
			}
			stats := corpus.ix.Stats()
			b.ReportMetric(float64(hits)/float64(idx), "hits/op")
			b.ReportMetric(float64(stats.DistinctTexts), "distinct-texts")
			b.ReportMetric(float64(stats.Words), "words")
			b.ReportMetric(corpus.build.Seconds(), "build-s")
			b.ReportMetric(float64(corpus.heap)/(1<<20), "index-MB")
		})
	}
}

// BenchmarkSet replaces the three texts of one of 10k IDs.
func BenchmarkSet(b *testing.B) {
	gen := synth.New(9, 50_000)
	updates := make([][]string, 100_000)
	for idx := range updates {
		updates[idx] = []string{gen.Text(), gen.Text(), gen.Text()}
	}
	for _, size := range []int{100_000, 1_000_000} {
		b.Run(fmt.Sprintf("texts=%d", size), func(b *testing.B) {
			corpus := benchIndex(b, size)
			var id uint32
			for b.Loop() {
				corpus.ix.Set(1<<31+id%10_000, updates[id%uint32(len(updates))]...)
				id++
			}
		})
	}
}
