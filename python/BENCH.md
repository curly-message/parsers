## Benchmark of curly-message-python

Node v24.22.3, linux x64. A time is per run: the median of 20 samples, and the range of the samples without their fastest and slowest quarter.

| Row | Kind | 1.0.0 | |
|---|---|--:|---|
| dist/*.whl | size | 78,258 B |  |
| installed curly_message/ | size | 192,851 B |  |
| host conversions: resolving five catalogue messages | count | 9 |  |
| host conversions: one value named 100 times | count | 1 |  |
| tails started: resolving five catalogue messages | count | 6 |  |
| text trees built: resolving five catalogue messages | count | 1 |  |
| str.join calls: 100 000 characters under 10 000 inline defaults | count | 1 |  |
| props entries enumerated: 100 selections over wrapper props of 1 000 entries | count | 1,000 |  |
| frames on the stack: one more read in progress | count | 3 |  |
| props texts converted: 100 numbers and 100 plurals over call props | count | 4 |  |
| resolve: five catalogue messages ×20 | time | 5.74 ms (5.63 ms–5.82 ms) |  |
| resolve: a message with no placeholder ×1000 | time | 231 µs (225 µs–237 µs) |  |
| resolve: a flat message ×200 | time | 1.07 ms (1.05 ms–1.09 ms) |  |
| resolve: a selection ×100 | time | 2.14 ms (2.13 ms–2.16 ms) |  |
| resolve: selections holding placeholders ×100 | time | 2.75 ms (2.7 ms–2.78 ms) |  |
| resolve: a list of 1 000 distinct leaves ×5 | time | 1.71 ms (1.67 ms–1.73 ms) |  |
| resolve: 5 000 placeholders | time | 16.9 ms (16.8 ms–17.1 ms) |  |
| resolve: nested 10 000 levels | time | 6.94 ms (6.82 ms–6.99 ms) |  |
| resolve: inline defaults nested 10 000 levels | time | 67.7 ms (67.3 ms–68.9 ms) |  |
| resolve: 100 000 characters under 10 000 inline defaults | time | 85.5 ms (84.6 ms–85.9 ms) |  |
| resolve: a placeholder of 10 000 blanks | time | 2.23 ms (2.18 ms–2.25 ms) |  |
| resolve: a key with 10 000 blanks inside | time | 1.88 ms (1.86 ms–1.9 ms) |  |
| resolve: 1 000 placeholders that never close ×5 | time | 3.52 ms (3.51 ms–3.54 ms) |  |
| resolve: 5 000 options | time | 7.39 ms (7.3 ms–7.48 ms) |  |
| resolve: an option key of 50 000 characters | time | 9.98 ms (9.78 ms–10 ms) |  |
| resolve: wrapper props of 5 000 entries | time | 684 µs (654 µs–705 µs) |  |
| resolve: 1 000 selections over wrapper props of 1 000 entries | time | 7.66 ms (7.6 ms–7.72 ms) |  |
| resolve: 1 000 host modifiers over wrapper props of 1 000 entries | time | 7.37 ms (7.33 ms–7.4 ms) |  |
| resolve: 200 wrappers that configure a host modifier over props of 1 000 entries | time | 2.2 ms (2.17 ms–2.22 ms) |  |
| resolve: a host modifier ×200 | time | 2.1 ms (2.09 ms–2.15 ms) |  |
| resolve: kept options read one inside another, to the reentry bound ×10 | time | 2.08 ms (2.06 ms–2.17 ms) |  |
| resolve: a number ×50 | time | 1.31 ms (1.18 ms–1.36 ms) |  |
| resolve: a plural ×50 | time | 1.63 ms (1.62 ms–1.66 ms) |  |
| resolve: 100 numbers and 100 plurals over call props ×5 | time | 19.8 ms (19.6 ms–19.9 ms) |  |
| resolve: an ago at a tag with extensions ×50 | time | 1.95 ms (1.91 ms–1.98 ms) |  |
| resolve: a date ×50 | time | 2.45 ms (2.42 ms–2.47 ms) |  |
| resolve: a date named in its zone ×50 | time | 3.05 ms (3.01 ms–3.27 ms) |  |
| cst: five catalogue messages ×20 | time | 1.81 ms (1.79 ms–1.82 ms) |  |
| cst: 5 000 placeholders | time | 17.9 ms (17.5 ms–18.2 ms) |  |
| cst: nested 250 levels | time | 1.32 ms (1.31 ms–1.38 ms) |  |
| import curly_message, in a fresh interpreter | time | 25.3 ms (24.9 ms–25.6 ms) |  |
| first Intl call, in a fresh interpreter | time | 65.7 ms (65.4 ms–65.8 ms) |  |

Passed; nothing was compared.
