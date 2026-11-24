## Benchmark of curly-message/parser

Node v24.22.3, linux x64. A time is per run: the median of 20 samples, and the range of the samples without their fastest and slowest quarter.

| Row | Kind | 1.0.0-RC.0 | |
|---|---|--:|---|
| src/ | size | 323,797 B |  |
| src/, gzipped | size | 76,942 B |  |
| host calls: resolving five catalogue messages over objects | count | 4 |  |
| modifier calls: resolving the catalogue and three messages a host modifier is on | count | 10 |  |
| bytes marked: 10 numbers taking turns between two wrappers, over a fraction digit option padded with 1 MiB of medium mathematical spaces | count | 2,228,138 |  |
| characters read: a number and a date of a value padded with 24 000 no-break spaces a side | count | 12 |  |
| peak bytes of the resolution: 50 numbers over wrappers each with a fraction digit option listing one text of 1 MiB twenty times | count | 20,994,208 |  |
| resolve: five catalogue messages, x 10 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 1.63 ms (1.6 ms–1.67 ms) |  |
| resolve: 5 000 placeholders, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 15 ms (14.8 ms–15 ms) |  |
| resolve: nested 10 000 levels, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 4.06 ms (3.99 ms–4.15 ms) |  |
| resolve: a fallback chain nested 10 000 levels, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 41.5 ms (41.1 ms–41.9 ms) |  |
| resolve: a placeholder of 10 000 blanks, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 753 µs (744 µs–795 µs) |  |
| resolve: a key with 10 000 blanks inside, x 10 (PHP 8.4.28, opcache on, JIT off) | time | 201 µs (196 µs–222 µs) |  |
| resolve: 1 000 placeholders that never close, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 449 µs (444 µs–471 µs) |  |
| resolve: 5 000 options, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 6.26 ms (6.2 ms–6.32 ms) |  |
| resolve: 5 000 placeholders in a fiber, beside 250 suspended holding options, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 27.5 ms (27.3 ms–27.9 ms) |  |
| resolve: an option key of 50 000 bytes, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 101 µs (77.7 µs–106 µs) |  |
| resolve: wrapper props of 5 000 entries, x 10 (PHP 8.4.28, opcache on, JIT off) | time | 109 µs (108 µs–115 µs) |  |
| resolve: 1 000 placeholders over props of 5 000 entries in a wrapper and in the call, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 6.9 ms (6.82 ms–6.97 ms) |  |
| resolve: 500 keys, each wrapped with a property of its own, over props of 1 000 entries in the call, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 3.07 ms (3 ms–3.16 ms) |  |
| resolve: 5 000 keys beginning with NUL, over an object of 5 000 entries, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 15.2 ms (15.1 ms–15.3 ms) |  |
| resolve: 1 000 comparisons of text outside ASCII, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 10.1 ms (10 ms–10.2 ms) |  |
| resolve: 1 000 numeric comparisons, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 11.3 ms (11.2 ms–11.5 ms) |  |
| resolve: a list of 1 000 records, serialized, x 10 (PHP 8.4.28, opcache on, JIT off) | time | 40.8 ms (40.6 ms–40.9 ms) |  |
| resolve: a list no conversion describes, read at 1 000 placeholders, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 5.22 ms (5.16 ms–5.29 ms) |  |
| resolve: a list with a hole, past what a conversion visits, read at 1 000 keys, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 5.72 ms (5.62 ms–5.77 ms) |  |
| resolve: an object past what a conversion visits, in a list read at 100 keys, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 2.91 ms (2.88 ms–2.94 ms) |  |
| resolve: 5 000 placeholders whose host modifier reads an option and the chain, in a fiber, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 62.8 ms (62.6 ms–63.2 ms) |  |
| Cst::parse: five catalogue messages, x 10 (PHP 8.4.28, opcache on, JIT off) | time | 1.31 ms (1.27 ms–1.33 ms) |  |
| Cst::parse: 5 000 placeholders, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 21.7 ms (21.3 ms–21.8 ms) |  |
| Cst::parse: nested 250 levels, x 1 (PHP 8.4.28, opcache on, JIT off) | time | 1.94 ms (1.91 ms–1.98 ms) |  |
| resolve: five catalogue messages at cs, formatters kept, x 10 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 1.58 ms (1.57 ms–1.6 ms) |  |
| resolve: five catalogue messages at cs, formatters built afresh, x 10 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 5.73 ms (5.71 ms–5.77 ms) |  |
| resolve: a number at cs, formatters built afresh, x 10 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 1.95 ms (1.86 ms–2.02 ms) |  |
| resolve: 1 000 numbers at cs over props of 5 000 entries, formatters kept, x 1 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 7.83 ms (7.77 ms–7.94 ms) |  |
| resolve: 100 numbers at cs, a fraction digit option padded with 2 000 blanks a side, x 1 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 2.36 ms (2.32 ms–2.42 ms) |  |
| resolve: 10 numbers at cs, a fraction digit option padded with 64 KiB of ideographic spaces a side, x 1 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 866 µs (854 µs–914 µs) |  |
| resolve: 10 numbers at cs, a fraction digit option padded with 64 KiB of medium mathematical spaces a side, x 1 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 1.13 ms (1.11 ms–1.16 ms) |  |
| resolve: a date at cs, formatters built afresh, x 10 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 1.73 ms (1.72 ms–1.73 ms) |  |
| resolve: a relative time at cs, formatters built afresh, x 10 (PHP 8.4.28, opcache on, JIT off, ICU 78.2) | time | 2.18 ms (2.16 ms–2.21 ms) |  |

Passed; nothing was compared.
