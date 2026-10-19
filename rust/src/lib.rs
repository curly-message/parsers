//! The Rust implementation of the [Curly Message Format].
//!
//! [Curly Message Format]: https://github.com/curly-message/spec
#![forbid(unsafe_code)]
#![warn(missing_docs)]

mod value;

pub use value::{Array, Object, Value};

#[cfg(doctest)]
#[doc = include_str!("../README.md")]
pub struct ReadmeDoctests;
