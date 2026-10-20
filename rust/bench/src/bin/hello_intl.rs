//! Resolves one message through each of the six Intl modifiers and every
//! `timeStyle`, and writes it: the program whose stripped binary the benchmark
//! weighs with `intl`, so the size row follows what the feature adds to a
//! host's binary when everything it offers is used.

use std::io::{self, Write};

use curly_message_parser::{Context, IcuIntl, Object, Parser, Value};

const INSTANT: f64 = 1_709_632_800_000.0;

fn timed(style: &str) -> Object {
    Object::new().with("value", Value::date(INSTANT)).with(
        "props",
        Object::new().with(
            "date",
            Object::new()
                .with("dateStyle", "full")
                .with("timeStyle", style)
                .with("timeZone", "Europe/Prague"),
        ),
    )
}

fn main() -> io::Result<()> {
    let payload = Object::new()
        .with("n", 1234.5678)
        .with("price", 2.5)
        .with("delta", -86_400_000)
        .with("files", 3)
        .with("short", timed("short"))
        .with("medium", timed("medium"))
        .with("long", timed("long"))
        .with("full", timed("full"));
    let props = Object::new().with("currency", Object::new().with("currency", "EUR"));
    let text = Parser::builder().intl(IcuIntl::new()).build().resolve(
        "{{n:number}} {{price:currency}} {{delta:ago}} \
         {{files:plural; one:file; other:files}} {{files:ordinal; one:st; two:nd; few:rd; other:th}}\n\
         {{short:date}}\n{{medium:date}}\n{{long:date}}\n{{full:date}}\n",
        &Context::new().payload(&payload).props(&props).locale("en"),
    );

    io::stdout().lock().write_all(text.as_bytes())
}
