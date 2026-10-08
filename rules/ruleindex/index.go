// Package ruleindex matches texts against many rule sets at once: the sets are indexed up front, the
// texts are not. Use it when there are many sets (up to millions of rules) and few texts at a time;
// for the opposite, see the textindex package.
package ruleindex

import (
	"cmp"
	"iter"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/Back-to-code/f2f-match-utils/rules"
)

type Match[ID comparable] struct {
	ID ID
	// Include is the index in the set's Includes of the first include that matches the text.
	Include int
	// Rule is that include as it was written.
	Rule string
}

type options struct {
	estimate Estimator
	// syncMergeWeight is the largest merge done inside a write; bigger ones run in the background.
	syncMergeWeight int
}

type Option func(*options)

// WithEstimator replaces HeuristicEstimate, for instance with pattern counts from the texts that will be
// matched. It only affects speed.
func WithEstimator(estimate Estimator) Option {
	return func(opts *options) { opts.estimate = estimate }
}

// Index holds rule sets by ID and matches texts against all of them. It is safe for concurrent use.
// Matching never waits for writes: it sees the sets as they were when it started.
//
// Sets live in immutable segments. A write adds a segment with the new sets and marks their previous
// versions dead; segments are merged in the background as they pile up, so a write costs about the
// work of compiling the sets it adds.
type Index[ID comparable] struct {
	opts    options
	current atomic.Pointer[snapshot[ID]]

	mu    sync.Mutex
	where map[ID]location[ID]
	seq   uint64
	// merging is the last segment of the run a background merge is replacing, nil when none runs.
	merging *segment[ID]
	idle    *sync.Cond
}

type segment[ID comparable] struct {
	core *core
	ids  []ID
	// seqs order matches by when their set was written.
	seqs    []uint64
	sources []rules.Set
	weight  int
}

type location[ID comparable] struct {
	seg   *segment[ID]
	local uint32
}

type view[ID comparable] struct {
	seg *segment[ID]
	// dead has a bit per set that was replaced or deleted; nil when none was. Snapshots share it, so it
	// is copied before it changes.
	dead       []uint64
	deadSets   int
	deadWeight int
}

func (v *view[ID]) liveSets() int   { return len(v.seg.ids) - v.deadSets }
func (v *view[ID]) liveWeight() int { return v.seg.weight - v.deadWeight }

func (v *view[ID]) isDead(local uint32) bool {
	return v.dead != nil && hasBit(v.dead, local)
}

type snapshot[ID comparable] struct {
	views []view[ID]
}

// setWeight is what a set costs to compile and match; empty sets weigh 1 so they still count.
func setWeight(set rules.Set) int { return 1 + len(set.Includes) + len(set.Excludes) }

func New[ID comparable](opts ...Option) *Index[ID] {
	ix := &Index[ID]{
		opts:  options{estimate: HeuristicEstimate, syncMergeWeight: 50_000},
		where: map[ID]location[ID]{},
	}
	for _, opt := range opts {
		opt(&ix.opts)
	}
	ix.idle = sync.NewCond(&ix.mu)
	ix.current.Store(&snapshot[ID]{})
	return ix
}

type entry[ID comparable] struct {
	id     ID
	seq    uint64
	source rules.Set
}

// buildSegment compiles the entries one at a time, so only one set's syntax trees are alive at once.
func buildSegment[ID comparable](estimate Estimator, entries []entry[ID], problems func(idx int, found []rules.Problem)) *segment[ID] {
	seg := &segment[ID]{ids: make([]ID, len(entries)), seqs: make([]uint64, len(entries)), sources: make([]rules.Set, len(entries))}
	builder := newCoreBuilder()
	for idx, entry := range entries {
		compiled, found := rules.Compile(entry.source)
		if problems != nil && len(found) > 0 {
			problems(idx, found)
		}
		builder.add(compiled)
		seg.ids[idx] = entry.id
		seg.seqs[idx] = entry.seq
		seg.sources[idx] = entry.source
		seg.weight += setWeight(entry.source)
	}
	seg.core = builder.finish(estimate)
	return seg
}

func cloneSet(set rules.Set) rules.Set {
	return rules.Set{Includes: slices.Clone(set.Includes), Excludes: slices.Clone(set.Excludes)}
}

// Set stores the set under id, replacing the set stored there before. Includes and excludes that do not
// parse are left out and returned; the others are indexed and keep their index in set.Includes.
func (ix *Index[ID]) Set(id ID, set rules.Set) []rules.Problem {
	var problems []rules.Problem
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.seq++
	seg := buildSegment(ix.opts.estimate, []entry[ID]{{id, ix.seq, cloneSet(set)}}, func(_ int, found []rules.Problem) { problems = found })
	ix.add(seg)
	return problems
}

// SetAll is Set for many sets, built into one segment; use it to fill an index. When an ID comes twice,
// the last set wins. The result holds the problems per ID, for IDs that have any.
func (ix *Index[ID]) SetAll(sets iter.Seq2[ID, rules.Set]) map[ID][]rules.Problem {
	var entries []entry[ID]
	position := map[ID]int{}
	for id, set := range sets {
		idx, ok := position[id]
		if ok {
			entries[idx].source = cloneSet(set)
			continue
		}
		position[id] = len(entries)
		entries = append(entries, entry[ID]{id: id, source: cloneSet(set)})
	}
	if len(entries) == 0 {
		return nil
	}
	problems := map[ID][]rules.Problem{}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	for idx := range entries {
		ix.seq++
		entries[idx].seq = ix.seq
	}
	seg := buildSegment(ix.opts.estimate, entries, func(idx int, found []rules.Problem) { problems[entries[idx].id] = found })
	ix.add(seg)
	return problems
}

// Delete removes the set stored under id and reports whether there was one.
func (ix *Index[ID]) Delete(id ID) bool {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	loc, ok := ix.where[id]
	if !ok {
		return false
	}
	delete(ix.where, id)
	draft := ix.draft()
	draft.kill(loc)
	ix.maintain(draft)
	ix.current.Store(&snapshot[ID]{views: draft.views})
	return true
}

// Get returns the set stored under id, as it was given to Set.
func (ix *Index[ID]) Get(id ID) (rules.Set, bool) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	loc, ok := ix.where[id]
	if !ok {
		return rules.Set{}, false
	}
	return cloneSet(loc.seg.sources[loc.local]), true
}

// Len returns the number of sets.
func (ix *Index[ID]) Len() int {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	return len(ix.where)
}

// Compact waits for a running background merge and then merges all segments into one, which makes
// matching as fast as it gets. Writes wait while it runs; matching does not.
func (ix *Index[ID]) Compact() {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.waitIdle()
	draft := ix.draft()
	if len(draft.views) > 1 || (len(draft.views) == 1 && draft.views[0].deadSets > 0) {
		ix.mergeNow(draft, 0, len(draft.views))
	}
	ix.dropEmpty(draft)
	ix.current.Store(&snapshot[ID]{views: draft.views})
}

func (ix *Index[ID]) waitIdle() {
	for ix.merging != nil {
		ix.idle.Wait()
	}
}

func (ix *Index[ID]) add(seg *segment[ID]) {
	draft := ix.draft()
	for local, id := range seg.ids {
		old, ok := ix.where[id]
		if ok {
			draft.kill(old)
		}
		ix.where[id] = location[ID]{seg, uint32(local)}
	}
	draft.views = append(draft.views, view[ID]{seg: seg})
	draft.owned = append(draft.owned, true)
	ix.maintain(draft)
	ix.current.Store(&snapshot[ID]{views: draft.views})
}

// draft is the next snapshot while a write prepares it; owned tells which dead bitsets it may change.
type draft[ID comparable] struct {
	views []view[ID]
	owned []bool
}

func (ix *Index[ID]) draft() *draft[ID] {
	views := slices.Clone(ix.current.Load().views)
	return &draft[ID]{views: views, owned: make([]bool, len(views))}
}

func (d *draft[ID]) find(seg *segment[ID]) int {
	for idx := len(d.views) - 1; idx >= 0; idx-- {
		if d.views[idx].seg == seg {
			return idx
		}
	}
	return -1
}

func (d *draft[ID]) kill(loc location[ID]) {
	idx := d.find(loc.seg)
	v := &d.views[idx]
	if !d.owned[idx] {
		dead := make([]uint64, (len(v.seg.ids)+63)/64)
		copy(dead, v.dead)
		v.dead = dead
		d.owned[idx] = true
	}
	if v.dead == nil {
		v.dead = make([]uint64, (len(v.seg.ids)+63)/64)
	}
	if hasBit(v.dead, loc.local) {
		return
	}
	v.dead[loc.local>>6] |= 1 << (loc.local & 63)
	v.deadSets++
	v.deadWeight += setWeight(v.seg.sources[loc.local])
}

func (d *draft[ID]) replace(lo, hi int, with view[ID]) {
	d.views = slices.Replace(d.views, lo, hi, with)
	d.owned = slices.Replace(d.owned, lo, hi, true)
}

// mergeFactor bounds how alike neighbouring segments may be: an older segment is merged with the newer
// ones once it holds less than mergeFactor times their live weight. Segment sizes then grow at least
// geometrically, so there are only a few of them.
const mergeFactor = 3

// maintain merges segments where the policy asks for it: small merges right away, a big one in the
// background. Segments a background merge is replacing are left alone.
func (ix *Index[ID]) maintain(d *draft[ID]) {
	for {
		ix.dropEmpty(d)
		start := 0
		if ix.merging != nil {
			start = d.find(ix.merging) + 1
		}
		lo, hi := pickMerge(d.views[start:])
		if lo < 0 {
			return
		}
		lo, hi = lo+start, hi+start
		weight := 0
		for idx := lo; idx < hi; idx++ {
			weight += d.views[idx].liveWeight()
		}
		if weight > ix.opts.syncMergeWeight && ix.merging == nil {
			ix.mergeInBackground(slices.Clone(d.views[lo:hi]))
			return
		}
		// While a background merge runs, the newest segments are still merged as far as that is cheap,
		// so writes in the meantime do not pile up into many segments.
		for lo < hi-1 && weight > ix.opts.syncMergeWeight {
			weight -= d.views[lo].liveWeight()
			lo++
		}
		if weight > ix.opts.syncMergeWeight || (hi-lo < 2 && d.views[lo].deadWeight <= d.views[lo].liveWeight()) {
			return
		}
		ix.mergeNow(d, lo, hi)
	}
}

// dropEmpty removes segments whose sets are all dead, except those a background merge is replacing.
func (ix *Index[ID]) dropEmpty(d *draft[ID]) {
	busy := -1
	if ix.merging != nil {
		busy = d.find(ix.merging)
	}
	for idx := len(d.views) - 1; idx > busy; idx-- {
		if d.views[idx].liveSets() == 0 {
			d.views = slices.Delete(d.views, idx, idx+1)
			d.owned = slices.Delete(d.owned, idx, idx+1)
		}
	}
}

// pickMerge returns the run of views to merge, or -1, -1. views are oldest first.
func pickMerge[ID comparable](views []view[ID]) (int, int) {
	last := len(views) - 1
	lo, newer := last, 0
	if last >= 0 {
		newer = views[last].liveWeight()
	}
	for lo > 0 && views[lo-1].liveWeight() < mergeFactor*newer {
		lo--
		newer += views[lo].liveWeight()
	}
	if lo < last {
		return lo, last + 1
	}
	// A segment that is mostly dead is rewritten on its own.
	for idx, v := range views {
		if v.deadWeight > v.liveWeight() {
			return idx, idx + 1
		}
	}
	return -1, -1
}

// buildMerged builds a segment from the live sets of the views and returns where each set came from.
func (ix *Index[ID]) buildMerged(inputs []view[ID]) (*segment[ID], []location[ID]) {
	var entries []entry[ID]
	var from []location[ID]
	for _, v := range inputs {
		for local, id := range v.seg.ids {
			if v.isDead(uint32(local)) {
				continue
			}
			entries = append(entries, entry[ID]{id, v.seg.seqs[local], v.seg.sources[local]})
			from = append(from, location[ID]{v.seg, uint32(local)})
		}
	}
	return buildSegment(ix.opts.estimate, entries, nil), from
}

// install puts a merged segment in place of the views it was built from. Sets written while it was
// built are dead in it: where no longer points at the place it took them from.
func (ix *Index[ID]) install(d *draft[ID], inputs []view[ID], seg *segment[ID], from []location[ID]) {
	lo := d.find(inputs[0].seg)
	merged := view[ID]{seg: seg}
	for local, id := range seg.ids {
		if ix.where[id] == from[local] {
			ix.where[id] = location[ID]{seg, uint32(local)}
			continue
		}
		if merged.dead == nil {
			merged.dead = make([]uint64, (len(seg.ids)+63)/64)
		}
		merged.dead[local>>6] |= 1 << (local & 63)
		merged.deadSets++
		merged.deadWeight += setWeight(seg.sources[local])
	}
	d.replace(lo, lo+len(inputs), merged)
}

func (ix *Index[ID]) mergeNow(d *draft[ID], lo, hi int) {
	inputs := slices.Clone(d.views[lo:hi])
	seg, from := ix.buildMerged(inputs)
	ix.install(d, inputs, seg, from)
}

func (ix *Index[ID]) mergeInBackground(inputs []view[ID]) {
	ix.merging = inputs[len(inputs)-1].seg
	go func() {
		seg, from := ix.buildMerged(inputs)
		ix.mu.Lock()
		defer ix.mu.Unlock()
		d := ix.draft()
		ix.install(d, inputs, seg, from)
		ix.merging = nil
		ix.maintain(d)
		ix.current.Store(&snapshot[ID]{views: d.views})
		ix.idle.Broadcast()
	}()
}

var normalizers = sync.Pool{New: func() any { return &rules.Normalizer{} }}

// Match returns every set the text matches, in the order the sets were last written.
func (ix *Index[ID]) Match(text string) []Match[ID] {
	normalizer := normalizers.Get().(*rules.Normalizer)
	defer normalizers.Put(normalizer)
	return ix.MatchWords(normalizer.Words(text))
}

// MatchWords is Match on a word set as returned by rules.Words.
func (ix *Index[ID]) MatchWords(words []string) []Match[ID] {
	type ordered struct {
		seq   uint64
		match Match[ID]
	}
	var found []ordered
	for _, v := range ix.current.Load().views {
		m := v.seg.core.matchers.Get().(*matcher)
		for _, hit := range m.matchWords(words) {
			if v.isDead(hit.set) {
				continue
			}
			found = append(found, ordered{v.seg.seqs[hit.set], Match[ID]{
				ID:      v.seg.ids[hit.set],
				Include: int(hit.include),
				Rule:    v.seg.sources[hit.set].Includes[hit.include],
			}})
		}
		v.seg.core.matchers.Put(m)
	}
	slices.SortFunc(found, func(left, right ordered) int { return cmp.Compare(left.seq, right.seq) })
	matches := make([]Match[ID], len(found))
	for idx := range found {
		matches[idx] = found[idx].match
	}
	return matches
}

type Stats struct {
	Sets     int
	Segments int
	// DeadSets are replaced or deleted sets still held by a segment until it is merged.
	DeadSets int
	Merging  bool
}

func (ix *Index[ID]) Stats() Stats {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	stats := Stats{Sets: len(ix.where), Merging: ix.merging != nil}
	for _, v := range ix.current.Load().views {
		stats.Segments++
		stats.DeadSets += v.deadSets
	}
	return stats
}
