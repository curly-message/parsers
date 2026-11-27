## Benchmark of curly-message-python

Node v24.22.3, linux x64. A time is per run: the median of 20 samples, and the range of the samples without their fastest and slowest quarter.

| Row | Kind | 1.0.0 | |
|---|---|--:|---|
| dist/*.whl | size | 78,706 B |  |
| installed curly_message_parser/ | size | 192,879 B |  |
| host conversions: resolving five catalogue messages | count | 9 |  |
| host conversions: one value named 100 times | count | 1 |  |
| tails started: resolving five catalogue messages | count | 6 |  |
| text trees built: resolving five catalogue messages | count | 1 |  |
| str.join calls: 100 000 characters under 10 000 inline defaults | count | 1 |  |
| props entries enumerated: 100 selections over wrapper props of 1 000 entries | count | 1,000 |  |
| frames on the stack: one more read in progress | count | 3 |  |
| props texts converted: 100 numbers and 100 plurals over call props | count | 4 |  |
| resolve: five catalogue messages ×20 | time | 8.42 ms (8.33 ms–8.51 ms) |  |
| resolve: a message with no placeholder ×1000 | time | 334 µs (327 µs–340 µs) |  |
| resolve: a flat message ×200 | time | 1.34 ms (1.33 ms–1.36 ms) |  |
| resolve: a selection ×100 | time | 3.67 ms (3.65 ms–3.7 ms) |  |
| resolve: selections holding placeholders ×100 | time | 4.54 ms (4.5 ms–4.57 ms) |  |
| resolve: a list of 1 000 distinct leaves ×5 | time | 1.94 ms (1.94 ms–1.96 ms) |  |
| resolve: 5 000 placeholders | time | 21.5 ms (21.4 ms–21.6 ms) |  |
| resolve: nested 10 000 levels | time | 8.48 ms (8.4 ms–8.54 ms) |  |
| resolve: inline defaults nested 10 000 levels | time | 83.9 ms (82.5 ms–84.7 ms) |  |
| resolve: 100 000 characters under 10 000 inline defaults | time | 104 ms (103 ms–105 ms) |  |
| resolve: a placeholder of 10 000 blanks | time | 2.47 ms (2.45 ms–2.48 ms) |  |
| resolve: a key with 10 000 blanks inside | time | 2.09 ms (2.09 ms–2.12 ms) |  |
| resolve: 1 000 placeholders that never close ×5 | time | 4.19 ms (4.14 ms–4.22 ms) |  |
| resolve: 5 000 options | time | 9 ms (8.8 ms–9.24 ms) |  |
| resolve: an option key of 50 000 characters | time | 10.6 ms (10.5 ms–10.7 ms) |  |
| resolve: wrapper props of 5 000 entries | time | 877 µs (817 µs–895 µs) |  |
| resolve: 1 000 selections over wrapper props of 1 000 entries | time | 11.8 ms (11.7 ms–11.8 ms) |  |
| resolve: 1 000 host modifiers over wrapper props of 1 000 entries | time | 10.3 ms (10.1 ms–10.5 ms) |  |
| resolve: 200 wrappers that configure a host modifier over props of 1 000 entries | time | 3.6 ms (3.57 ms–3.65 ms) |  |
| resolve: a host modifier ×200 | time | 3.92 ms (3.87 ms–3.97 ms) |  |
| resolve: kept options read one inside another, to the reentry bound ×10 | time | 3.28 ms (3.25 ms–3.33 ms) |  |
| resolve: a number ×50 | time | 2.64 ms (2.63 ms–2.65 ms) |  |
| resolve: a plural ×50 | time | 3.4 ms (3.36 ms–3.42 ms) |  |
| resolve: 100 numbers and 100 plurals over call props ×5 | time | 40.3 ms (40.1 ms–40.9 ms) |  |
| resolve: an ago at a tag with extensions ×50 | time | 4.04 ms (3.98 ms–4.1 ms) |  |
| resolve: a date ×50 | time | 4.05 ms (4.01 ms–4.1 ms) |  |
| resolve: a date named in its zone ×50 | time | 4.99 ms (4.96 ms–5.04 ms) |  |
| cst: five catalogue messages ×20 | time | 2.36 ms (2.35 ms–2.37 ms) |  |
| cst: 5 000 placeholders | time | 22.9 ms (22.7 ms–23.1 ms) |  |
| cst: nested 250 levels | time | 1.81 ms (1.72 ms–1.89 ms) |  |
| import curly_message_parser, in a fresh interpreter | time | 31.1 ms (31 ms–31.5 ms) |  |
| first Intl call, in a fresh interpreter | time | 83.5 ms (83.3 ms–83.7 ms) |  |

Passed; nothing was compared.
