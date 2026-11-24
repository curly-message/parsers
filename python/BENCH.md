## Benchmark of curly-message-python

Node v24.22.3, linux x64. A time is per run: the median of 20 samples, and the range of the samples without their fastest and slowest quarter.

| Row | Kind | 1.0.0rc0 | |
|---|---|--:|---|
| dist/*.whl | size | 79,807 B |  |
| installed curly_message/ | size | 196,405 B |  |
| host conversions: resolving five catalogue messages | count | 9 |  |
| host conversions: one value named 100 times | count | 1 |  |
| tails started: resolving five catalogue messages | count | 6 |  |
| text trees built: resolving five catalogue messages | count | 1 |  |
| str.join calls: 100 000 characters under 10 000 inline defaults | count | 1 |  |
| props entries enumerated: 100 selections over wrapper props of 1 000 entries | count | 1,000 |  |
| frames on the stack: one more read in progress | count | 3 |  |
| props texts converted: 100 numbers and 100 plurals over call props | count | 4 |  |
| resolve: five catalogue messages ×20 | time | 8.26 ms (8.04 ms–8.4 ms) |  |
| resolve: a message with no placeholder ×1000 | time | 255 µs (253 µs–266 µs) |  |
| resolve: a flat message ×200 | time | 1.15 ms (1.14 ms–1.19 ms) |  |
| resolve: a selection ×100 | time | 3.01 ms (2.99 ms–3.06 ms) |  |
| resolve: selections holding placeholders ×100 | time | 3.85 ms (3.83 ms–3.89 ms) |  |
| resolve: a list of 1 000 distinct leaves ×5 | time | 1.63 ms (1.62 ms–1.65 ms) |  |
| resolve: 5 000 placeholders | time | 18.7 ms (18.5 ms–18.8 ms) |  |
| resolve: nested 10 000 levels | time | 7.09 ms (7.01 ms–7.15 ms) |  |
| resolve: inline defaults nested 10 000 levels | time | 72.8 ms (72 ms–73.3 ms) |  |
| resolve: 100 000 characters under 10 000 inline defaults | time | 90.7 ms (88.8 ms–91.1 ms) |  |
| resolve: a placeholder of 10 000 blanks | time | 2.06 ms (2.04 ms–2.08 ms) |  |
| resolve: a key with 10 000 blanks inside | time | 1.74 ms (1.72 ms–1.76 ms) |  |
| resolve: 1 000 placeholders that never close ×5 | time | 3.65 ms (3.63 ms–3.68 ms) |  |
| resolve: 5 000 options | time | 7.34 ms (7.3 ms–7.39 ms) |  |
| resolve: an option key of 50 000 characters | time | 8.9 ms (8.89 ms–8.98 ms) |  |
| resolve: wrapper props of 5 000 entries | time | 717 µs (697 µs–761 µs) |  |
| resolve: 1 000 selections over wrapper props of 1 000 entries | time | 8.42 ms (8.38 ms–8.48 ms) |  |
| resolve: 1 000 host modifiers over wrapper props of 1 000 entries | time | 8.19 ms (8.14 ms–8.31 ms) |  |
| resolve: 200 wrappers that configure a host modifier over props of 1 000 entries | time | 2.58 ms (2.55 ms–2.59 ms) |  |
| resolve: a host modifier ×200 | time | 2.77 ms (2.75 ms–2.79 ms) |  |
| resolve: kept options read one inside another, to the reentry bound ×10 | time | 2.75 ms (2.73 ms–2.77 ms) |  |
| resolve: a number ×50 | time | 2.39 ms (2.35 ms–2.43 ms) |  |
| resolve: a plural ×50 | time | 3.07 ms (3.07 ms–3.1 ms) |  |
| resolve: 100 numbers and 100 plurals over call props ×5 | time | 37.1 ms (36.9 ms–37.2 ms) |  |
| resolve: an ago at a tag with extensions ×50 | time | 3.7 ms (3.68 ms–3.71 ms) |  |
| resolve: a date ×50 | time | 3.73 ms (3.71 ms–3.75 ms) |  |
| resolve: a date named in its zone ×50 | time | 4.82 ms (4.77 ms–4.88 ms) |  |
| cst: five catalogue messages ×20 | time | 2 ms (1.99 ms–2.01 ms) |  |
| cst: 5 000 placeholders | time | 18.9 ms (18.6 ms–19.3 ms) |  |
| cst: nested 250 levels | time | 1.48 ms (1.46 ms–1.58 ms) |  |
| import curly_message, in a fresh interpreter | time | 29.2 ms (29 ms–29.3 ms) |  |
| first Intl call, in a fresh interpreter | time | 77.2 ms (76.7 ms–77.6 ms) |  |

Passed; nothing was compared.
