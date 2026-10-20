//! Measures the rows of the benchmark that `bench/rows.mjs` lists, so that the
//! family's harness, which runs on Node, measures the parser's release build:
//!
//! ```text
//! curly-message-bench list          one JSON object per row: {"name": ..., "kind": ...}
//! curly-message-bench count <row>   the row's count
//! curly-message-bench serve <row>   sets the row up and writes a line; then, for
//!                                   each byte read, runs one batch of the row and
//!                                   writes a line, until its input closes
//! ```
//!
//! A time row runs a batch of a fixed number of iterations, the same on both
//! sides of a comparison and named in the row, so the round trip over the pipe
//! is a constant both sides pay and a small fraction of what is measured.
#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::arithmetic_side_effects
)]
// Counting allocations takes a global allocator, and installing one takes an
// unsafe impl.
#![allow(unsafe_code)]

use std::alloc::{GlobalAlloc, Layout, System};
use std::fmt;
use std::hint::black_box;
use std::io::{self, Read, Write};
use std::process::ExitCode;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use curly_message_parser::{Context, IcuIntl, Object, Parser, Value, cst};

struct Counting;

static ALLOCATIONS: AtomicUsize = AtomicUsize::new(0);

// SAFETY: every call is forwarded to the system allocator unchanged; the count
// is an atomic, which allocates nothing.
unsafe impl GlobalAlloc for Counting {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        ALLOCATIONS.fetch_add(1, Ordering::Relaxed);
        // SAFETY: the caller's contract is passed on as it is.
        unsafe { System.alloc(layout) }
    }

    unsafe fn dealloc(&self, ptr: *mut u8, layout: Layout) {
        // SAFETY: the caller's contract is passed on as it is.
        unsafe { System.dealloc(ptr, layout) }
    }

    unsafe fn realloc(&self, ptr: *mut u8, layout: Layout, new_size: usize) -> *mut u8 {
        ALLOCATIONS.fetch_add(1, Ordering::Relaxed);
        // SAFETY: the caller's contract is passed on as it is.
        unsafe { System.realloc(ptr, layout, new_size) }
    }
}

#[global_allocator]
static GLOBAL: Counting = Counting;

// Messages as a catalogue holds them: interpolation, a selection, a plural, a
// placeholder an option value writes, and a formatted number and date.
const MESSAGES: [&str; 5] = [
    "Hello, {{name}}!",
    "{{name}} has {{count:number}} new {{count:plural; one:message; other:messages}}.",
    "{{gender; male:He; female:She; default:They}} replied to {{thread}}.",
    "{{count:plural; one:{{count}} file; few:{{count}} files; other:{{count}} files}} in {{folder; :the root; default:{{folder}}}}.",
    "Total: {{amount:number}}, due {{due:date}}.",
];

// 2026-01-31T00:00:00Z.
const DUE: f64 = 1_769_817_600_000.0;

fn catalogue_payload() -> Object {
    Object::new()
        .with("name", "Ada")
        .with("count", 3)
        .with("gender", "female")
        .with("thread", "Plans")
        .with("folder", "docs")
        .with("amount", 1234.5)
        .with("due", Value::date(DUE))
}

fn intl_parser() -> Parser {
    Parser::builder().intl(IcuIntl::new()).build()
}

fn nested(depth: usize) -> String {
    format!("{}x{}", "{{v; a:".repeat(depth), "}}".repeat(depth))
}

fn flat(count: usize) -> String {
    (0..count)
        .map(|index| format!("{{{{p{index}}}}}"))
        .collect::<Vec<_>>()
        .join(" ")
}

// One iteration of a time row: resolving `message` over `payload`, in `en`.
fn resolving(parser: Parser, message: String, payload: Object) -> Box<dyn FnMut()> {
    Box::new(move || {
        black_box(parser.resolve(
            message.as_str(),
            &Context::new().payload(&payload).locale("en"),
        ));
    })
}

fn describing(message: String) -> Box<dyn FnMut()> {
    Box::new(move || {
        black_box(cst(&message));
    })
}

// The allocations `run` makes, run once beforehand so that what is set up on
// first use is not counted.
fn allocations(mut run: impl FnMut()) -> f64 {
    run();

    let before = ALLOCATIONS.load(Ordering::Relaxed);

    run();

    (ALLOCATIONS.load(Ordering::Relaxed) - before) as f64
}

// A host value that counts how often it is asked to describe itself.
struct Counted(Arc<AtomicUsize>);

impl fmt::Display for Counted {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.0.fetch_add(1, Ordering::Relaxed);
        formatter.write_str("h")
    }
}

enum Measure {
    Count(fn() -> f64),
    // Sets the row up and returns one iteration, run `batch` times a batch.
    Time {
        batch: usize,
        setup: fn() -> Box<dyn FnMut()>,
    },
}

struct Row {
    name: &'static str,
    measure: Measure,
}

impl Row {
    fn kind(&self) -> &'static str {
        match self.measure {
            Measure::Count(_) => "count",
            Measure::Time { .. } => "time",
        }
    }

    fn title(&self) -> String {
        match self.measure {
            Measure::Count(_) => self.name.to_owned(),
            Measure::Time { batch, .. } => format!("{}, {}", self.name, times(batch)),
        }
    }
}

fn times(count: usize) -> String {
    if count == 1 {
        return "once".to_owned();
    }

    let digits = count.to_string();
    let mut text = String::new();

    for (index, digit) in digits.chars().enumerate() {
        if index > 0 && (digits.len() - index).is_multiple_of(3) {
            text.push(' ');
        }

        text.push(digit);
    }

    text + " times"
}

const ROWS: &[Row] = &[
    Row {
        name: "allocations: resolving five catalogue messages",
        measure: Measure::Count(|| {
            let parser = intl_parser();
            let payload = catalogue_payload();

            allocations(|| {
                for message in MESSAGES {
                    black_box(
                        parser.resolve(message, &Context::new().payload(&payload).locale("en")),
                    );
                }
            })
        }),
    },
    Row {
        name: "allocations: resolving nested 10 000 levels",
        measure: Measure::Count(|| {
            let parser = Parser::new();
            let payload = Object::new().with("v", "a");
            let message = nested(10_000);

            allocations(|| {
                black_box(parser.resolve(message.as_str(), &Context::new().payload(&payload)));
            })
        }),
    },
    Row {
        name: "allocations: cst of five catalogue messages",
        measure: Measure::Count(|| {
            allocations(|| {
                for message in MESSAGES {
                    black_box(cst(message));
                }
            })
        }),
    },
    Row {
        name: "allocations: a payload of 100 owned strings",
        measure: Measure::Count(|| {
            let keys: Vec<String> = (0..100).map(|index| format!("p{index}")).collect();

            allocations(|| {
                black_box(
                    keys.iter()
                        .map(|key| (key.as_str(), key.clone()))
                        .collect::<Object>(),
                );
            })
        }),
    },
    Row {
        name: "allocations: 100 placeholders a modifier answers with an owned string",
        measure: Measure::Count(|| {
            let parser = Parser::builder()
                .modifier("echo", |input: &curly_message_parser::ModifierInput<'_>| {
                    Ok(Some(Value::from(input.value().to_owned())))
                })
                .build();
            let payload = Object::new().with("v", "a");
            let message = "{{v:echo}}".repeat(100);

            allocations(|| {
                black_box(parser.resolve(message.as_str(), &Context::new().payload(&payload)));
            })
        }),
    },
    Row {
        name: "Display calls: one value read by 1 000 placeholders",
        measure: Measure::Count(|| {
            let calls = Arc::new(AtomicUsize::new(0));
            let payload = Object::new().with("h", Value::host(Counted(Arc::clone(&calls))));
            let message = "{{h}}".repeat(1_000);

            black_box(Parser::new().resolve(message.as_str(), &Context::new().payload(&payload)));

            calls.load(Ordering::Relaxed) as f64
        }),
    },
    Row {
        name: "resolve: five catalogue messages",
        measure: Measure::Time {
            batch: 1_000,
            setup: || {
                let parser = intl_parser();
                let payload = catalogue_payload();

                Box::new(move || {
                    for message in MESSAGES {
                        black_box(
                            parser.resolve(message, &Context::new().payload(&payload).locale("en")),
                        );
                    }
                })
            },
        },
    },
    Row {
        name: "resolve: 5 000 placeholders",
        measure: Measure::Time {
            batch: 10,
            setup: || resolving(Parser::new(), flat(5_000), Object::new()),
        },
    },
    Row {
        name: "resolve: nested 10 000 levels",
        measure: Measure::Time {
            batch: 10,
            setup: || resolving(Parser::new(), nested(10_000), Object::new().with("v", "a")),
        },
    },
    Row {
        name: "resolve: a placeholder of 10 000 blanks",
        measure: Measure::Time {
            batch: 100,
            setup: || {
                resolving(
                    Parser::new(),
                    format!("{{{{{}}}}}", " ".repeat(10_000)),
                    Object::new(),
                )
            },
        },
    },
    Row {
        name: "resolve: a key with 10 000 blanks inside",
        measure: Measure::Time {
            batch: 100,
            setup: || {
                resolving(
                    Parser::new(),
                    format!("{{{{a{}b}}}}", " ".repeat(10_000)),
                    Object::new(),
                )
            },
        },
    },
    Row {
        name: "resolve: 1 000 placeholders that never close",
        measure: Measure::Time {
            batch: 10,
            setup: || resolving(Parser::new(), "{{a; c:".repeat(1_000), Object::new()),
        },
    },
    Row {
        name: "resolve: 5 000 options",
        measure: Measure::Time {
            batch: 10,
            setup: || {
                resolving(
                    Parser::new(),
                    format!("{{{{a{}}}}}", "; a".repeat(5_000)),
                    Object::new().with("a", "a"),
                )
            },
        },
    },
    Row {
        name: "resolve: an option key of 50 000 characters",
        measure: Measure::Time {
            batch: 100,
            setup: || {
                resolving(
                    Parser::new(),
                    format!("{{{{a; {}:v}}}}", "x".repeat(50_000)),
                    Object::new().with("a", "a"),
                )
            },
        },
    },
    Row {
        name: "resolve: wrapper props of 5 000 entries",
        measure: Measure::Time {
            batch: 1_000,
            setup: || {
                let parser = Parser::builder()
                    .modifier("test", |input: &curly_message_parser::ModifierInput<'_>| {
                        Ok(Some(Value::from(input.value())))
                    })
                    .build();
                let entries: Object = (0..5_000)
                    .map(|index| (format!("p{index}"), index))
                    .collect();
                let payload = Object::new().with(
                    "v",
                    Object::new()
                        .with("value", "a")
                        .with("props", Object::new().with("test", entries)),
                );
                let props = Object::new().with("test", Object::new().with("q", 1));

                Box::new(move || {
                    black_box(parser.resolve(
                        "{{v:test}}",
                        &Context::new().payload(&payload).props(&props).locale("en"),
                    ));
                })
            },
        },
    },
    Row {
        name: "resolve: an object of 2 000 short strings, serialized",
        measure: Measure::Time {
            batch: 100,
            setup: || {
                let object: Object = (0..2_000)
                    .map(|index| (format!("key{index}"), format!("value \"{index}\"\n")))
                    .collect();

                resolving(
                    Parser::new(),
                    "{{o}}".to_owned(),
                    Object::new().with("o", object),
                )
            },
        },
    },
    Row {
        name: "resolve: 100 000 nested inline defaults past the nesting limit",
        measure: Measure::Time {
            batch: 1,
            setup: || {
                resolving(
                    Parser::new(),
                    format!(
                        "{}x{}",
                        "{{a; default:".repeat(100_000),
                        "}}".repeat(100_000)
                    ),
                    Object::new(),
                )
            },
        },
    },
    Row {
        name: "cst: five catalogue messages",
        measure: Measure::Time {
            batch: 1_000,
            setup: || {
                Box::new(|| {
                    for message in MESSAGES {
                        black_box(cst(message));
                    }
                })
            },
        },
    },
    Row {
        name: "cst: 5 000 placeholders",
        measure: Measure::Time {
            batch: 10,
            setup: || describing(flat(5_000)),
        },
    },
    Row {
        name: "cst: nested 250 levels",
        measure: Measure::Time {
            batch: 100,
            setup: || describing(nested(250)),
        },
    },
];

fn serve(batch: usize, setup: fn() -> Box<dyn FnMut()>) -> io::Result<()> {
    let mut op = setup();
    let mut input = io::stdin().lock();
    let mut output = io::stdout().lock();
    let mut byte = [0u8];

    output.write_all(b"\n")?;
    output.flush()?;

    while input.read(&mut byte)? == 1 {
        for _ in 0..batch {
            op();
        }

        output.write_all(b"\n")?;
        output.flush()?;
    }

    Ok(())
}

fn run(args: &[String]) -> Result<(), String> {
    let mut output = io::stdout().lock();

    match args {
        [command] if command == "list" => {
            for row in ROWS {
                // Row names hold no character JSON escapes.
                writeln!(
                    output,
                    "{{\"name\":\"{}\",\"kind\":\"{}\"}}",
                    row.title(),
                    row.kind()
                )
                .map_err(|error| error.to_string())?;
            }

            Ok(())
        }
        [command, name] if command == "count" || command == "serve" => {
            let row = ROWS
                .iter()
                .find(|row| row.title() == *name)
                .ok_or_else(|| format!("no row is named {name:?}"))?;

            match (command.as_str(), &row.measure) {
                ("count", Measure::Count(count)) => {
                    writeln!(output, "{}", count()).map_err(|error| error.to_string())
                }
                ("serve", Measure::Time { batch, setup }) => {
                    drop(output);
                    serve(*batch, *setup).map_err(|error| error.to_string())
                }
                _ => Err(format!("the row {name:?} is no {command} row")),
            }
        }
        _ => Err("usage: curly-message-bench list | count <row> | serve <row>".to_owned()),
    }
}

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();

    match run(&args) {
        Ok(()) => ExitCode::SUCCESS,
        Err(error) => {
            let _ = writeln!(io::stderr(), "curly-message-bench: {error}");
            ExitCode::FAILURE
        }
    }
}
