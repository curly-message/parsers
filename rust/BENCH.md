## Benchmark of curly-message-parser

Node v24.22.3, linux x64. A time is per run: the median of 20 samples, and the range of the samples without their fastest and slowest quarter.

| Row | Kind | 1.0.0 | |
|---|---|--:|---|
| curly-message-parser.crate | size | 83,214 B |  |
| hello: a program resolving one message, stripped | size | 520,392 B |  |
| hello-intl: a program resolving through every Intl modifier, stripped | size | 7,578,640 B |  |
| hello-intl, gzipped | size | 2,375,484 B |  |
| crates: the parser's dependency tree | count | 1 |  |
| crates: the parser's dependency tree with intl | count | 64 |  |
| allocations: resolving five catalogue messages | count | 71 |  |
| allocations: resolving nested 10 000 levels | count | 56 |  |
| allocations: resolving four numbers written as text | count | 25 |  |
| allocations: cst of five catalogue messages | count | 72 |  |
| allocations: a payload of 100 owned strings | count | 202 |  |
| allocations: 100 placeholders a modifier answers with an owned string | count | 312 |  |
| Display calls: one value read by 1 000 placeholders | count | 1 |  |
| Display calls: one prop read by 100 placeholders | count | 1 |  |
| stack bytes: per level of a modifier reading an option | count | 2,656 |  |
| stack bytes: per resolution a modifier starts | count | 3,456 |  |
| resolve: five catalogue messages, 1 000 times | time | 9.34 ms (9.32 ms–9.37 ms) |  |
| resolve: four numbers written as text, 10 000 times | time | 11.8 ms (11.7 ms–11.9 ms) |  |
| resolve: 5 000 placeholders, 10 times | time | 6.57 ms (6.5 ms–6.64 ms) |  |
| resolve: nested 10 000 levels, 10 times | time | 8.1 ms (8.08 ms–8.13 ms) |  |
| resolve: a placeholder of 10 000 blanks, 100 times | time | 5.02 ms (5.02 ms–5.03 ms) |  |
| resolve: a key with 10 000 blanks inside, 100 times | time | 3.41 ms (3.4 ms–3.45 ms) |  |
| resolve: 1 000 placeholders that never close, 10 times | time | 774 µs (771 µs–782 µs) |  |
| resolve: 5 000 options, 10 times | time | 1.23 ms (1.22 ms–1.24 ms) |  |
| resolve: an option key of 50 000 characters, 100 times | time | 14.3 ms (14.3 ms–14.3 ms) |  |
| resolve: wrapper props of 5 000 entries, 1 000 times | time | 252 µs (245 µs–255 µs) |  |
| resolve: an object of 2 000 short strings, serialized, 100 times | time | 7.15 ms (7.12 ms–7.17 ms) |  |
| resolve: 100 000 nested inline defaults past the nesting limit, once | time | 20.5 ms (20.4 ms–20.6 ms) |  |
| resolve: 250 placeholders, a custom modifier reading its own option two levels deep, 100 times | time | 19 ms (19 ms–19 ms) |  |
| cst: five catalogue messages, 1 000 times | time | 3.83 ms (3.78 ms–3.87 ms) |  |
| cst: 5 000 placeholders, 10 times | time | 5.6 ms (5.57 ms–5.64 ms) |  |
| cst: nested 250 levels, 100 times | time | 4.3 ms (4.28 ms–4.33 ms) |  |

Passed; nothing was compared.
