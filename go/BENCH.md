## Benchmark of github.com/curly-message/parsers/go

Node v24.22.3, linux x64. A time is per run: the median of 20 samples, and the range of the samples without their fastest and slowest quarter.

| Row | Kind | 1.0.0-next.0 | |
|---|---|--:|---|
| dist/size: a program resolving one message, stripped | size | 2,216,098 B |  |
| dist/size, gzipped | size | 982,189 B |  |
| allocations: resolving five catalogue messages with a stand-in Intl | count | 165 |  |
| bytes allocated: resolving a short plain message | count | 672 |  |
| String calls: one value read by 100 placeholders | count | 1 |  |
| allocations: resolving a message that reports, told to OnReport | count | 12 |  |
| CurlyText calls: one Texter value read by 100 placeholders | count | 1 |  |
| allocations: one placeholder more reading wrapper props of 5 000 entries | count | 5 |  |
| allocations: one placeholder more composing alternating wrapper props over a caller's text | count | 11 |  |
| resolve: five catalogue messages with a stand-in Intl, 1 000 times | time | 11.6 ms (11.4 ms–11.7 ms) |  |
| resolve: a short plain message, 10 000 times | time | 4.49 ms (4.45 ms–4.55 ms) |  |
| resolve: a host modifier reading its option, 10 000 times | time | 12 ms (11.8 ms–12.2 ms) |  |
| resolve: a message that reports, told to OnReport, 10 000 times | time | 6.84 ms (6.76 ms–6.9 ms) |  |
| resolve: a Texter value resolving a message, 10 000 times | time | 14.4 ms (14.2 ms–14.6 ms) |  |
| resolve: 5 000 placeholders, 10 times | time | 11.3 ms (11.1 ms–11.5 ms) |  |
| resolve: nested 10 000 levels, 10 times | time | 4.78 ms (4.62 ms–5.01 ms) |  |
| resolve: a placeholder of 10 000 blanks, 100 times | time | 7.6 ms (7.54 ms–7.68 ms) |  |
| resolve: a key with 10 000 blanks inside, 100 times | time | 6.01 ms (5.95 ms–6.13 ms) |  |
| resolve: 1 000 placeholders that never close, 10 times | time | 536 µs (532 µs–545 µs) |  |
| resolve: 5 000 options, 10 times | time | 4.92 ms (4.89 ms–5 ms) |  |
| resolve: an option key of 50 000 characters, 100 times | time | 32.3 ms (32.1 ms–32.4 ms) |  |
| resolve: wrapper props of 5 000 entries, 10 times | time | 4.38 ms (4.34 ms–4.42 ms) |  |
| resolve: 1 000 placeholders reading wrapper props of 5 000 entries, 10 times | time | 9.08 ms (9 ms–9.16 ms) |  |
| resolve: 100 000 nested inline defaults past the nesting limit, once | time | 40.7 ms (39.5 ms–42.2 ms) |  |
| resolve: a plain object of 5 000 members, 10 times | time | 15.9 ms (15.7 ms–16 ms) |  |
| resolve: a plain array of 5 000 structs, each written by encoding/json, 10 times | time | 83.2 ms (82.3 ms–83.9 ms) |  |
| resolve: an array nested 99 999 levels, once | time | 42.8 ms (39.3 ms–45.5 ms) |  |
| resolve: a struct of 1 000 fields, as fmt prints it, 10 times | time | 779 µs (770 µs–797 µs) |  |
| resolve: a DAG of 25 levels, given up at the node limit, 10 times | time | 215 ms (215 ms–216 ms) |  |
| resolve: a map of 5 000 array keys, as fmt prints it, 10 times | time | 52.3 ms (52.1 ms–52.8 ms) |  |
| resolve: a map of 10 000 keys of 256 numbers, as fmt prints it, 10 times | time | 972 ms (970 ms–978 ms) |  |
| resolve: a map of 70 000 keys of 256 numbers, sorted, then refused past the text bound, once | time | 474 ms (467 ms–484 ms) |  |
| resolve: a map of 49 keys of 400 000 zero-size elements a method prints, sorted, once | time | 2.69 s (2.66 s–2.72 s) |  |
| resolve: a map of 50 keys of 400 000 zero-size elements a method prints, refused at the count, once | time | 451 ms (451 ms–452 ms) |  |
| resolve: a named slice shared over 21 levels, as fmt prints it, once | time | 292 ms (283 ms–295 ms) |  |
| resolve: 20 000 bytes of text around one placeholder, 100 times | time | 2.37 ms (2.34 ms–2.38 ms) |  |
| cst: five catalogue messages, 1 000 times | time | 21.8 ms (21.6 ms–22.2 ms) |  |
| cst: 5 000 placeholders, 10 times | time | 71.9 ms (68 ms–75.5 ms) |  |
| cst: nested 250 levels, 100 times | time | 33.1 ms (32 ms–33.6 ms) |  |
| cst: the encoding of a tree nested 100 000 levels, once | time | 47.7 ms (47.5 ms–49.1 ms) |  |

Passed; nothing was compared.
