# `f2f-match-utils`

_This repo is public intentionally_

This is a shared repo that holds go packages mainly for text matching.

| package | what |
| --- | --- |
| [`rules`](rules) | a pattern language for matching short texts, with validation, explanations and two indexes: [`rules/textindex`](rules/textindex) for few rules against many texts and [`rules/ruleindex`](rules/ruleindex) for many rules against few texts |
| [`tokens`](tokens) | converts text into word tokens for fast matching |
| [`sanitize`](sanitize) | string normalisation and escaping helpers |
| [`language`](language) | resolves language names and codes to ISO 639-3 |
