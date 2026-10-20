//! The Rust implementation of the [Curly Message Format].
//!
//! [Curly Message Format]: https://github.com/curly-message/spec
#![forbid(unsafe_code)]
#![warn(missing_docs)]

mod builtins;
mod convert;
mod cst;
mod depth;
mod modifier;
mod number;
mod parser;
mod props;
mod report;
mod scan;
mod syntax;
mod value;
mod walk;

pub use cst::{Children, NodeKind, NodeRef, Tree, cst};
pub use modifier::{Modifier, ModifierError, ModifierInput};
pub use parser::{Context, Level, Message, Parser, ParserBuilder};
pub use props::Props;
pub use report::{Code, Origin, Report};
pub use value::{Array, Object, Value};

/// The most bytes of UTF-8 of placeholder results a resolution's output
/// carries (section 13). The message's own text is not counted against it and
/// always renders.
pub const MAX_OUTPUT_LENGTH: usize = 100_000;

/// The most bytes of UTF-8 of value text a resolution reads (section 13).
pub const MAX_READ_LENGTH: usize = 100_000;

/// The most nodes one conversion of a value visits (section 13).
pub const MAX_CONVERSION_NODES: usize = 100_000;

/// The most code units of UTF-16 one serialization of a value writes, as V8
/// counts the length of a string, and so the most a value's `Display` may
/// write. One that would write more describes nothing, as one past
/// [`MAX_CONVERSION_NODES`] does: a value naming one large child many times
/// visits few nodes and could still write without bound. The number is the
/// most a string holds in V8, where the JavaScript implementation meets it.
pub const MAX_SERIALIZED_LENGTH: usize = 536_870_888;

/// How deeply a placeholder may nest and still resolve (section 13).
pub const MAX_NESTING: usize = 8;

/// The most bytes of a message a report carries.
pub const MAX_REPORTED_LENGTH: usize = 120;

/// The most walk activations one thread holds at once, across every
/// resolution and every parser running on it.
pub const MAX_HOST_DEPTH: usize = 16;

#[cfg(doctest)]
#[doc = include_str!("../README.md")]
pub struct ReadmeDoctests;
