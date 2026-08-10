# language

Resolves a language written any way a human or a machine might write it — a name in Dutch,
English or the language itself, an alias, an ISO 639-1 or ISO 639-2 code — to one ISO 639-3
code.

```go
import "github.com/Back-to-code/f2f-match-utils/language"

code, ok := language.Parse("Nederlandse Gebarentaal") // dse
code, ok := language.Parse("nl")                      // nld, the ISO 639-1 code is upgraded
name := language.SupportedLanguages[code]             // the Dutch name, if it has one
```

The list covers all of ISO 639-3: **7,928 languages**, reachable under **18,514 names**. Only
**244** carry a Dutch name, which is what marks a language as supported, and **185** have an
ISO 639-1 code.

## The three files

| File | Hand edited | What it is |
| --- | --- | --- |
| `languages.tsv` | yes | the language list, the only thing to edit |
| `tables_gen.go` | no | the lookup tables, written by `go generate` |
| `internal/` | yes | the generator and the encoding both sides share |

```
languages.tsv  ──  go generate ./language/...  ──▶  tables_gen.go
```

Edit the TSV, run the generator, commit both. A stale `tables_gen.go` fails
`TestGeneratedTablesAreUpToDate`.

## Why a TSV and not Go

The list used to be a Go file: 8005 lines of `supportedLanguage` structs holding every
name as a string. It worked, but everything in it was paid for at runtime.

That table was 634 KB of static data containing **31,716 pointers**, all of which the
garbage collector walked on every cycle. `init()` then spent **3.36 ms** and 19,501
allocations building two maps out of it, which sat on the heap for the life of the process.
All of that to answer a question that is asked a handful of times per import.

A TSV is not compiled. It cannot end up in the binary by accident, it greps like text, it
diffs a line per language instead of a line per Go struct, and the compiler never has to
parse 18,514 string literals.

| | Go table | TSV + generated tables |
| --- | --- | --- |
| binary | +802 KB | **+133 KB** |
| writable, GC scanned data (`__data`) | 659 KB | **0** |
| heap after `init` | 1161 KB | **18 KB** |
| `init()` | 3.36 ms, 19,501 allocs | **no `init` at all** |
| `Parse` | 133.5 ns | **51.5 ns** |
| cold compile | 1.49 s | 0.77 s |

## Why the names are stored as hashes

A language name is only ever *matched against*. Nothing reads "Zuojiang Zhuang" back out
of the table and shows it to anyone — anything that needs a name to display reads
`SupportedLanguages`, the 244 Dutch names, which are kept as real strings, and nothing else.
So the table stores what a name hashes to, not the name:

```
nameHashes  [18514]uint32   FNV-1a of the normalized name, sorted ascending
nameCodes   [18514]uint16   the packed code that hash resolves to
```

A lookup normalizes the input, hashes it, and binary searches. Both arrays are plain
numbers, so they live in the binary as static data: no pointers for the collector to walk,
no `init` to build them, no heap.

**Collisions.** 32 bits over 18,514 names is comfortable — the expected number of
collisions is about 0.04 — and there are none today. That is not left to chance: the
generator refuses to write a table with a collision in it and prints both names, and
`TestNoHashCollisions` asserts the same. If a new language ever collides, rename or drop
one of its aliases, or widen the hash to 64 bits at the cost of another 74 KB.

**What this costs.** A hash is one way, so the table cannot answer "what names does this
language have". Nothing needs that. If something ever does, read `languages.tsv`, which is
what the tests do.

## Why `Code` is a `uint16`

An ISO 639-3 code is three letters out of an alphabet of 26, so there are 26³ = 17,576 of
them and they fit in a `uint16` with room to spare. `Code` is that base 26 number, shifted
up by one — `"aaa"` is a real code (Ghotuo), so the unshifted zero has to stay free to mean
"no language at all".

The point is not the byte saved over `[3]byte`. It is that a `Code` can no longer hold
something that is not a code. When it was three bytes it could hold `"NLD"`, or `"nl"` with
a trailing zero, and validation had to defend against both. Now only `Parse` and `CodeOf`
produce one, and neither hands back a malformed value.

`CodeOf` is the constructor for codes written out in source. It normalizes what is only a
spelling difference and rejects everything else:

```go
language.CodeOf("nld")   // nld
language.CodeOf("NLD")   // nld, case is not part of a code
language.CodeOf("n.l.d") // nld, non-letters are skipped
language.CodeOf("nl")    // zero
language.CodeOf("n/a")   // zero, "na" is not three letters
```

That last one is the reason non-letters are skipped but the result still has to be exactly
three letters. Padding or truncating `"n/a"` would quietly produce a *different real
language*, which is a far worse answer than none.

`CodeOf` only checks the shape. Use `Parse` to also check that a language by that code
exists.

## Why row order in the TSV matters

Two languages can want the same name. The ISO register hands the same reference name or
autonym to a macrolanguage and its varieties, and a three letter name sometimes spells
another language's code outright. The first row to claim a name keeps it, so the order of
`languages.tsv` is priority order: the languages most likely to be looked up are listed
first and win over the long tail.

Codes are claimed before any name, so a code always beats a name:

- `"yi"` is the ISO 639-1 code of Jiddisch, and also the Dutch name of Sichuan Yi. It
  resolves to Jiddisch.
- `"mei"` is the Dutch month, and also the ISO 639-3 code of Midob. It resolves to Midob,
  because language fields do hold bare codes.
- `"may"` is the ISO 639-2/B code of Malay, not a typo for `"mei"`.

The generator enforces the part of this that is ours to get right: a supported language must
own its Dutch name and its aliases, and must stay reachable by at least one of its names.
Losing one of those fails generation.

## Editing `languages.tsv`

Six tab separated columns. `#` comments and blank lines are ignored.

```
iso639_3 ⇥ iso639_1 ⇥ dutch ⇥ english ⇥ native ⇥ alias1,alias2
nld      ⇥ nl       ⇥ Nederlands ⇥ Dutch ⇥ Nederlands ⇥ dut
zzj      ⇥          ⇥            ⇥ Zuojiang Zhuang ⇥ ⇥
```

- **dutch** — set it only for languages that belong in the supported set. It is what
  `SupportedLanguages` maps to.
- **aliases** — spellings the three names do not already cover. They must be written
  normalized: lowercase, ASCII, single spaces, no punctuation. `sanitize.NormalizeString`
  is what decides.

Then run:

```sh
go generate ./language/...
```

The generator fails, loudly and with line numbers, on: a malformed row, a duplicate code or
Dutch name, an alias that is not normalized or that repeats a name, an offered language
losing its own name or becoming unreachable, and any hash collision.
