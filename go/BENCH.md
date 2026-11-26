## Benchmark of github.com/curly-message/parsers/go

Node v24.22.3, linux x64. A time is per run: the median of 20 samples, and the range of the samples without their fastest and slowest quarter.

| Row | Kind | 1.0.0 | |
|---|---|--:|---|
| dist/size: a program resolving one message, stripped | size | 2,216,098 B |  |
| dist/size, gzipped | size | 982,220 B |  |
| allocations: resolving five catalogue messages with a stand-in Intl | count | 165 |  |
| bytes allocated: resolving a short plain message | count | 672 |  |
| String calls: one value read by 100 placeholders | count | 1 |  |
| allocations: resolving a message that reports, told to OnReport | count | 12 |  |
| CurlyText calls: one Texter value read by 100 placeholders | count | 1 |  |
| allocations: one placeholder more reading wrapper props of 5 000 entries | count | 5 |  |
| allocations: one placeholder more composing alternating wrapper props over a caller's text | count | 11 |  |
| resolve: five catalogue messages with a stand-in Intl, 1 000 times | time | 11.4 ms (11.3 ms–11.6 ms) |  |
| resolve: a short plain message, 10 000 times | time | 4.52 ms (4.46 ms–4.61 ms) |  |
| resolve: a host modifier reading its option, 10 000 times | time | 11.8 ms (11.7 ms–12.1 ms) |  |
| resolve: a message that reports, told to OnReport, 10 000 times | time | 7.06 ms (6.8 ms–7.43 ms) |  |
| resolve: a Texter value resolving a message, 10 000 times | time | 14.5 ms (14.2 ms–14.7 ms) |  |
| resolve: 5 000 placeholders, 10 times | time | 11.3 ms (11.2 ms–11.5 ms) |  |
| resolve: nested 10 000 levels, 10 times | time | 4.79 ms (4.7 ms–4.91 ms) |  |
| resolve: a placeholder of 10 000 blanks, 100 times | time | 7.37 ms (7.3 ms–7.47 ms) |  |
| resolve: a key with 10 000 blanks inside, 100 times | time | 5.97 ms (5.92 ms–6.09 ms) |  |
| resolve: 1 000 placeholders that never close, 10 times | time | 554 µs (542 µs–568 µs) |  |
| resolve: 5 000 options, 10 times | time | 4.94 ms (4.9 ms–4.99 ms) |  |
| resolve: an option key of 50 000 characters, 100 times | time | 32 ms (31.9 ms–32.2 ms) |  |
| resolve: wrapper props of 5 000 entries, 10 times | time | 4.4 ms (4.33 ms–4.46 ms) |  |
| resolve: 1 000 placeholders reading wrapper props of 5 000 entries, 10 times | time | 9.04 ms (9 ms–9.07 ms) |  |
| resolve: 100 000 nested inline defaults past the nesting limit, once | time | 41.3 ms (39.8 ms–42.8 ms) |  |
| resolve: a plain object of 5 000 members, 10 times | time | 15.9 ms (15.8 ms–16.1 ms) |  |
| resolve: a plain array of 5 000 structs, each written by encoding/json, 10 times | time | 82.4 ms (81.8 ms–82.6 ms) |  |
| resolve: an array nested 99 999 levels, once | time | 40 ms (38.7 ms–43.6 ms) |  |
| resolve: a struct of 1 000 fields, as fmt prints it, 10 times | time | 777 µs (769 µs–790 µs) |  |
| resolve: a DAG of 25 levels, given up at the node limit, 10 times | time | 220 ms (218 ms–223 ms) |  |
| resolve: a map of 5 000 array keys, as fmt prints it, 10 times | time | 52.3 ms (52.2 ms–52.8 ms) |  |
| resolve: a map of 10 000 keys of 256 numbers, as fmt prints it, 10 times | time | 975 ms (972 ms–982 ms) |  |
| resolve: a map of 70 000 keys of 256 numbers, sorted, then refused past the text bound, once | time | 446 ms (440 ms–462 ms) |  |
| resolve: a map of 49 keys of 400 000 zero-size elements a method prints, sorted, once | time | 2.68 s (2.64 s–2.76 s) |  |
| resolve: a map of 50 keys of 400 000 zero-size elements a method prints, refused at the count, once | time | 451 ms (451 ms–452 ms) |  |
| resolve: a named slice shared over 21 levels, as fmt prints it, once | time | 286 ms (283 ms–294 ms) |  |
| resolve: 20 000 bytes of text around one placeholder, 100 times | time | 2.4 ms (2.36 ms–2.42 ms) |  |
| cst: five catalogue messages, 1 000 times | time | 22.2 ms (21.8 ms–23.2 ms) |  |
| cst: 5 000 placeholders, 10 times | time | 75.5 ms (74.2 ms–79.5 ms) |  |
| cst: nested 250 levels, 100 times | time | 32 ms (31.7 ms–32.5 ms) |  |
| cst: the encoding of a tree nested 100 000 levels, once | time | 49 ms (48.6 ms–50 ms) |  |

Passed; nothing was compared.
