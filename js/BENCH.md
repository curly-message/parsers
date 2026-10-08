## Benchmark of @curly-message/parser

Node v24.21.0, linux x64. A time is per run: the median of 20 samples, and the range of the samples without their fastest and slowest quarter.

| Row | Kind | 3.1.2 | |
|---|---|--:|---|
| dist/index.js | size | 15,576 B |  |
| dist/index.js, gzipped | size | 6,188 B |  |
| payload reads: resolving five catalogue messages | count | 22 |  |
| checker: instantiations of resolve calls against a payload type of 20 keys | count | 832 |  |
| resolve: five catalogue messages | time | 72.3 µs (54.5 µs–99.4 µs) |  |
| resolve: 5 000 placeholders | time | 6.43 ms (6.31 ms–6.56 ms) |  |
| resolve: nested 10 000 levels | time | 2.42 ms (2.39 ms–2.49 ms) |  |
| resolve: a placeholder of 10 000 blanks | time | 374 µs (373 µs–375 µs) |  |
| resolve: a key with 10 000 blanks inside | time | 337 µs (335 µs–345 µs) |  |
| resolve: 1 000 placeholders that never close | time | 214 µs (213 µs–220 µs) |  |
| resolve: 5 000 options | time | 3.11 ms (2.77 ms–4.1 ms) |  |
| resolve: an option key of 50 000 characters | time | 1.76 ms (1.76 ms–1.77 ms) |  |
| resolve: wrapper props of 5 000 entries | time | 917 µs (913 µs–923 µs) |  |
| cst: five catalogue messages | time | 24.7 µs (24.2 µs–24.7 µs) |  |
| cst: 5 000 placeholders | time | 4.2 ms (4.18 ms–4.24 ms) |  |
| cst: nested 250 levels | time | 332 µs (319 µs–334 µs) |  |
| createExtractor: five catalogue messages | time | 31.8 µs (30.2 µs–34.8 µs) |  |
| createExtractor: nested 250 levels | time | 377 µs (366 µs–387 µs) |  |

Passed.
