//! Resolves one message and writes it: the program whose stripped binary the
//! benchmark weighs, so the size row follows what the parser adds to a host's
//! binary without `intl`.

use std::io::{self, Write};

use curly_message_parser::{Context, Object, Parser};

fn main() -> io::Result<()> {
    let payload = Object::new().with("name", "Ada");
    let text = Parser::new().resolve(
        "Hello, {{name; default:Guest;}}!\n",
        &Context::new().payload(&payload),
    );

    io::stdout().lock().write_all(text.as_bytes())
}
