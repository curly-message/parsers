## Benchmark of @curly-message/parser

Node v24.22.3, linux x64. A time is per run: the median of 20 samples, and the range of the samples without their fastest and slowest quarter.

| Row | Kind | 3.2.0 | |
|---|---|--:|---|
| dist/index.js | size | 17,970 B |  |
| dist/index.js, gzipped | size | 7,191 B |  |
| payload reads: resolving five catalogue messages | count | 22 |  |
| props reads: 100 placeholders of each of three modifiers over one props slice of 1 000 entries | count | 900,300 |  |
| checker: instantiations of resolve calls against a payload type of 20 keys | count | 832 |  |
| resolve: five catalogue messages | time | 20.9 µs (20.5 µs–23.4 µs) |  |
| resolve: five catalogue messages of plain text | time | 1.34 µs (1.33 µs–1.34 µs) |  |
| resolve: five catalogue messages, compiled | time | 19.8 µs (19.6 µs–20.7 µs) |  |
| resolve: 2 000 catalogue messages, more than a parser keeps | time | 17.2 ms (16.4 ms–25.9 ms) |  |
| resolve: 2 000 catalogue messages, compiled | time | 8.47 ms (8.03 ms–13.3 ms) |  |
| resolve: five catalogue messages, each new to the parser | time | 46.4 µs (39.3 µs–66 µs) |  |
| resolve: a stream of short messages, none seen before | time | 1.21 µs (1.2 µs–1.25 µs) |  |
| resolve: a message of 12 000 characters, a new string each call | time | 50.9 µs (50.4 µs–51.6 µs) |  |
| resolve: 5 000 placeholders | time | 3.79 ms (3.72 ms–3.91 ms) |  |
| resolve: nested 10 000 levels | time | 2.29 ms (2.21 ms–2.54 ms) |  |
| resolve: inline defaults nested 1 000 levels | time | 1.25 ms (1.23 ms–1.29 ms) |  |
| resolve: inline defaults nested 1 000 levels, each writing text | time | 1.95 ms (1.8 ms–2.05 ms) |  |
| resolve: a placeholder of 10 000 blanks | time | 317 µs (314 µs–323 µs) |  |
| resolve: a key with 10 000 blanks inside | time | 285 µs (283 µs–286 µs) |  |
| resolve: 1 000 placeholders that never close | time | 210 µs (206 µs–212 µs) |  |
| resolve: 5 000 options | time | 2.13 ms (2.08 ms–2.92 ms) |  |
| resolve: an option key of 50 000 characters | time | 1.46 ms (1.4 ms–1.48 ms) |  |
| resolve: wrapper props of 5 000 entries | time | 957 µs (937 µs–979 µs) |  |
| resolve: a value of 4 000 nodes, serialized | time | 272 µs (271 µs–272 µs) |  |
| resolve: one formatted number, no props | time | 2.69 µs (2.61 µs–5.47 µs) |  |
| resolve: a selection and a formatted number, props for the call | time | 4.2 µs (4.15 µs–4.35 µs) |  |
| resolve: 250 placeholders, a custom modifier reading its own option two levels deep | time | 630 µs (581 µs–1.17 ms) |  |
| cst: five catalogue messages | time | 17.5 µs (17.4 µs–17.6 µs) |  |
| cst: 5 000 placeholders | time | 2.86 ms (2.8 ms–3.93 ms) |  |
| cst: nested 250 levels | time | 212 µs (205 µs–222 µs) |  |
| createExtractor: five catalogue messages | time | 21.2 µs (20.9 µs–21.3 µs) |  |
| createExtractor: nested 250 levels | time | 200 µs (199 µs–202 µs) |  |

Passed; nothing was compared.
