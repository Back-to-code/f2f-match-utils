package ruleindex

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/Back-to-code/f2f-match-utils/rules"
	"github.com/Back-to-code/f2f-match-utils/rules/internal/synth"
)

// Run: go test -run '^$' -bench . -benchmem ./rules/ruleindex (the 1M rule case builds for a while).

const benchIncludes = 50

type benchRules struct {
	ix    *Index[uint32]
	build time.Duration
	heap  uint64
}

var benchIndexes = map[int]*benchRules{}

func heapInUse() uint64 {
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

// benchIndex holds sets of benchIncludes includes and 3 excludes, size includes in all.
func benchIndex(b *testing.B, size int) *benchRules {
	b.Helper()
	if built, ok := benchIndexes[size]; ok {
		return built
	}
	gen := synth.New(7, size/10)
	before := heapInUse()
	start := time.Now()
	ix := New[uint32]()
	ix.SetAll(func(yield func(uint32, rules.Set) bool) {
		for id := range uint32(size / benchIncludes) {
			if !yield(id, gen.Set(benchIncludes, 3)) {
				return
			}
		}
	})
	built := &benchRules{ix: ix, build: time.Since(start)}
	built.heap = heapInUse() - before
	benchIndexes[size] = built
	return built
}

func BenchmarkMatch(b *testing.B) {
	gen := synth.New(8, 50_000)
	texts := make([]string, 10_000)
	for idx := range texts {
		texts[idx] = gen.Text()
	}
	for _, size := range []int{100_000, 1_000_000} {
		b.Run(fmt.Sprintf("rules=%d", size), func(b *testing.B) {
			built := benchIndex(b, size)
			matches, idx := 0, 0
			for b.Loop() {
				matches += len(built.ix.Match(texts[idx%len(texts)]))
				idx++
			}
			b.ReportMetric(float64(matches)/float64(idx), "matches/op")
			b.ReportMetric(built.build.Seconds(), "build-s")
			b.ReportMetric(float64(built.heap)/(1<<20), "index-MB")
		})
	}
}

// BenchmarkSet replaces one set in an index of 1M rules; merges run as they would in use.
func BenchmarkSet(b *testing.B) {
	built := benchIndex(b, 1_000_000)
	gen := synth.New(9, 50_000)
	updates := make([]rules.Set, 1000)
	for idx := range updates {
		updates[idx] = gen.Set(benchIncludes, 3)
	}
	var id uint32
	for b.Loop() {
		built.ix.Set(id%2000, updates[id%uint32(len(updates))])
		id++
	}
	stats := built.ix.Stats()
	b.ReportMetric(float64(stats.Segments), "segments")
}
