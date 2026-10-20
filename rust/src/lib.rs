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
pub use depth::ReentryLimit;
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

/// The most resolutions in progress at once on one thread, across every
/// parser running on it (section 13). A thread that runs out of stack aborts
/// the process, so this is the bound that stands in for the host's own limit
/// on a resolution reaching itself. A resolution asked for while this many
/// are in progress on its thread is refused: [`Parser::resolve`] unwinds with
/// [`ReentryLimit`] into the host code that asked, or, where the thread
/// already unwinds, answers the empty string and fails the host code that
/// asked all the same. One asked for where none is in progress never is.
///
/// What the crate keeps for this bound is the operating-system thread's,
/// which falls short of section 13's count per thread of execution for a
/// host that runs stackful fibers. Fibers that share an OS thread share its
/// count, so a resolution can be refused on a fiber where none is in
/// progress. They share whether the thread unwinds, and the crate's mark of
/// a refusal made while it does, so while one fiber is suspended in a `Drop`
/// that runs as it unwinds, a refusal on any of them answers the empty
/// string, and one made there can fail a host call on another fiber in
/// place of the one that asked. A fiber suspended in the middle of a
/// resolution holds a value on its stack that is not `Send`, so whatever
/// sends it to another OS thread, a host's `unsafe impl Send` or a coroutine
/// library's own, claims a `Send` the stack does not have: the resolution
/// stays counted on the thread it started on, and lowers the count of the
/// thread it finishes on.
pub const MAX_REENTRY: usize = 8;

#[cfg(doctest)]
#[doc = include_str!("../README.md")]
pub struct ReadmeDoctests;
