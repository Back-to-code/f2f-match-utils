# `sanitize`

String normalisation shared by every package that compares texts, plus escaping helpers. Each
function makes one pass over its input.

| function | what |
| --- | --- |
| `Fold` | lowercases, removes diacritics (`é` → `e`, `ß` → `ss`) and drops `.` and apostrophes; keeps everything else |
| `SplitWords` | the words of a text: runs of `a-z0-9` after `Fold` |
| `AppendWords` | `SplitWords` into buffers the caller reuses |
| `WordKey` | the words of `SplitWords` joined by single spaces; texts with the same key are the same to every match rule |
| `NormalizeString` | letters and digits only, folded, single spaces; keeps letters that have no ASCII form, e.g. Cyrillic |

## The fold table

`fold_table.go` maps every non-ASCII code point to what it folds to. It is generated from the Unicode
tables of the Go toolchain:

```sh
go generate ./sanitize/...
```

`TestFoldTableIsUpToDate` regenerates it when the toolchain has the same Unicode version as the table.
