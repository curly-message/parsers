//! A runner for the conformance set of the Curly Message Format, in Rust.
//!
//! The set's `RUNNER.md` states what a runner does; this crate does it for an
//! implementation in Rust, through an [`Adapter`] the implementation's tests
//! supply. It carries release 4.1.0 of the set, checked against the release's
//! digest before anything in it is read, and depends on no implementation:
//! inputs are decoded into the neutral [`Value`], which an adapter maps into
//! its own.
//!
//! [`plan`] lists every case the adapter's claims require, each ready to run,
//! beside the cases left out and why; [`run`] executes a plan and sorts the
//! outcomes; [`summarize`] writes them as the JavaScript runner does; and
//! [`audit`] puts the catalogue of deliberately wrong adapters to this runner,
//! so a runner that compares nothing is seen to.

#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::string_slice,
    clippy::arithmetic_side_effects
)]

mod adapter;
mod audit;
mod cases;
mod host;
pub mod js;
mod plan;
mod set;
mod tree;
mod value;

use std::fmt;

pub use adapter::{
    Adapter, Answer, BehaviourInput, CstClaim, ExposedRequest, Limits, RNode, Report, Resolution,
    Resolved, TreeAnswer, Unexpressible, behave,
};
pub use audit::{AuditOutcome, Audited, Defect, Mutated, audit};
pub use cases::FormatRequest;
pub use host::{FormatHost, GoldenHost, Recorded};
pub use plan::{
    Failure, Identity, Options, Outcome, Plan, Planned, RunResult, Skipped, check, plan, run,
    summarize,
};
pub use set::{
    Api, Behaviour, CATEGORIES, Case, Categories, Code, ConcreteCase, DIGEST, DefectEntry,
    Expected, ExpectedFormat, ExpectedNode, ExpectedReport, FORMAT, Fixture, FixtureFile,
    GeneratedCase, Level, Origin, Set, TreeCase, VERSION, Verdict, load, read_fixture, vendored,
};
pub use value::{Value, decode, nodes, plain};

/// Why a run was refused: a claim outside the format's vocabulary, a set that
/// is not the one released, or a case that is the set's defect rather than the
/// implementation's. A run that is refused reports no number, because a
/// number for it would mean nothing.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct RunError {
    message: String,
}

impl RunError {
    /// An error saying `message`.
    pub fn new(message: impl Into<String>) -> Self {
        Self {
            message: message.into(),
        }
    }

    /// What went wrong.
    pub fn message(&self) -> &str {
        &self.message
    }
}

impl fmt::Display for RunError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&self.message)
    }
}

impl std::error::Error for RunError {}
