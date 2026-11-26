## Benchmark of curly-message/parser

Node v24.22.3, linux x64. A time is per run: the median of 20 samples, and the range of the samples without their fastest and slowest quarter.

| Row | Kind | 1.0.0 | |
|---|---|--:|---|
| src/ | size | 318,514 B |  |
| src/, gzipped | size | 75,799 B |  |
| host calls: resolving five catalogue messages over objects | count | 4 |  |
| modifier calls: resolving the catalogue and three messages a host modifier is on | count | 10 |  |
| bytes marked: 10 numbers taking turns between two wrappers, over a fraction digit option padded with 1 MiB of medium mathematical spaces | count | 2,228,138 |  |
| characters read: a number and a date of a value padded with 24 000 no-break spaces a side | count | 12 |  |
| peak bytes of the resolution: 50 numbers over wrappers each with a fraction digit option listing one text of 1 MiB twenty times | count | 20,994,152 |  |
| resolve: five catalogue messages, x 10 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 2.43 ms (2.36 ms–2.46 ms) |  |
| resolve: 5 000 placeholders, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 24.2 ms (23.5 ms–24.3 ms) |  |
| resolve: nested 10 000 levels, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 6.93 ms (6.83 ms–7.01 ms) |  |
| resolve: a fallback chain nested 10 000 levels, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 63.6 ms (63.2 ms–63.9 ms) |  |
| resolve: a placeholder of 10 000 blanks, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 1.23 ms (1.21 ms–1.24 ms) |  |
| resolve: a key with 10 000 blanks inside, x 10 (PHP 8.4.28, opcache on, JIT off) | time | 288 µs (266 µs–296 µs) |  |
| resolve: 1 000 placeholders that never close, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 767 µs (752 µs–789 µs) |  |
| resolve: 5 000 options, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 10.1 ms (9.96 ms–10.4 ms) |  |
| resolve: 5 000 placeholders in a fiber, beside 250 suspended holding options, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 42.6 ms (42.3 ms–42.9 ms) |  |
| resolve: an option key of 50 000 bytes, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 157 µs (148 µs–161 µs) |  |
| resolve: wrapper props of 5 000 entries, x 10 (PHP 8.4.28, opcache on, JIT off) | time | 189 µs (187 µs–203 µs) |  |
| resolve: 1 000 placeholders over props of 5 000 entries in a wrapper and in the call, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 11.3 ms (11.2 ms–11.4 ms) |  |
| resolve: 500 keys, each wrapped with a property of its own, over props of 1 000 entries in the call, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 4.44 ms (4.42 ms–4.49 ms) |  |
| resolve: 5 000 keys beginning with NUL, over an object of 5 000 entries, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 23.8 ms (23.6 ms–24.4 ms) |  |
| resolve: 1 000 comparisons of text outside ASCII, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 17.1 ms (16.9 ms–17.4 ms) |  |
| resolve: 1 000 numeric comparisons, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 18.9 ms (18.4 ms–19.2 ms) |  |
| resolve: a list of 1 000 records, serialized, x 10 (PHP 8.4.28, opcache on, JIT off) | time | 68.3 ms (68.2 ms–68.5 ms) |  |
| resolve: a list no conversion describes, read at 1 000 placeholders, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 8.01 ms (7.97 ms–8.06 ms) |  |
| resolve: a list with a hole, past what a conversion visits, read at 1 000 keys, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 8.89 ms (8.83 ms–9.04 ms) |  |
| resolve: an object past what a conversion visits, in a list read at 100 keys, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 4.85 ms (4.83 ms–4.88 ms) |  |
| resolve: 5 000 placeholders whose host modifier reads an option and the chain, in a fiber, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 110 ms (109 ms–111 ms) |  |
| Cst::parse: five catalogue messages, x 10 (PHP 8.4.28, opcache on, JIT off) | time | 2.01 ms (1.93 ms–2.01 ms) |  |
| Cst::parse: 5 000 placeholders, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 34.2 ms (32.6 ms–35 ms) |  |
| Cst::parse: nested 250 levels, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 2.99 ms (2.96 ms–2.99 ms) |  |
| resolve: five catalogue messages at cs, formatters kept, x 10 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 2.37 ms (2.36 ms–2.39 ms) |  |
| resolve: five catalogue messages at cs, formatters built afresh, x 10 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 8.58 ms (8.56 ms–8.62 ms) |  |
| resolve: a number at cs, formatters built afresh, x 10 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 3.03 ms (2.97 ms–3.05 ms) |  |
| resolve: 1 000 numbers at cs over props of 5 000 entries, formatters kept, x 1 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 12.6 ms (12.4 ms–12.7 ms) |  |
| resolve: 100 numbers at cs, a fraction digit option padded with 2 000 blanks a side, x 1 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 4.51 ms (4.47 ms–4.52 ms) |  |
| resolve: 10 numbers at cs, a fraction digit option padded with 64 KiB of ideographic spaces a side, x 1 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 1.53 ms (1.52 ms–1.56 ms) |  |
| resolve: 10 numbers at cs, a fraction digit option padded with 64 KiB of medium mathematical spaces a side, x 1 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 1.93 ms (1.92 ms–1.94 ms) |  |
| resolve: a date at cs, formatters built afresh, x 10 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 2.91 ms (2.88 ms–2.94 ms) |  |
| resolve: a relative time at cs, formatters built afresh, x 10 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 3.36 ms (3.32 ms–3.4 ms) |  |

Passed; nothing was compared.
