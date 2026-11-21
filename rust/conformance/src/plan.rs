use crate::RunError;
use crate::adapter::{Adapter, Unexpressible};
use crate::cases;
use crate::host::FormatHost;
use crate::js;
use crate::set::{
    Api, Case, ConcreteCase, FORMAT, Fixture, FixtureFile, Keeping, Level, Set, TreeCase, vendored,
};
use crate::tree::{self, UNITS};
use crate::value::{Value, plain, refuses};

/// How a run is made.
#[derive(Clone, Copy, Default)]
pub struct Options<'a> {
    /// The set to run; the vendored release where it is `None`.
    pub set: Option<&'a Set>,
    /// The levels to run, a subset of those the adapter claims; all of them
    /// where it is `None`.
    pub levels: Option<&'a [&'a str]>,
    /// The runner's own host, which performs a case's request where the
    /// adapter exposes none.
    pub host: Option<&'a dyn FormatHost>,
}

/// Where a case is, and what it pins.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Identity {
    /// The case's id.
    pub id: String,
    /// The fixture file.
    pub file: String,
    /// The level of a resolution case.
    pub level: Option<Level>,
    /// The document a tree case pins, which is `CST.md`.
    pub document: Option<&'static str>,
    /// The section the case pins.
    pub section: String,
    /// What it pins.
    pub description: String,
}

/// A case left out, and why.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Skipped {
    /// The case.
    pub identity: Identity,
    /// Why it is left out.
    pub reason: String,
}

/// Why a case failed: the first mismatch, with each side written as JSON
/// writes it.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Failure {
    /// What differs.
    pub reason: String,
    /// What the case expected.
    pub expected: String,
    /// What the implementation produced.
    pub actual: String,
}

/// What became of a case that ran.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum Outcome {
    /// It passed.
    Passed,
    /// It passed, and the adapter supplied no reports to compare.
    PassedReportsUnobserved,
    /// It failed.
    Failed(Failure),
}

enum Work<'a> {
    Concrete(&'a ConcreteCase),
    Generated(&'a crate::set::GeneratedCase),
    Tree(&'a TreeCase),
}

/// A case ready to run.
pub struct Planned<'a> {
    /// The case.
    pub identity: Identity,
    adapter: &'a dyn Adapter,
    host: Option<&'a dyn FormatHost>,
    work: Work<'a>,
}

impl Planned<'_> {
    /// Runs the case. Its inputs are decoded here, on the thread that runs
    /// it. An `Err` is the set's defect, or an adapter that changed its
    /// claims since the plan was made, and ends the run.
    pub fn execute(&self) -> Result<Outcome, RunError> {
        match self.work {
            Work::Concrete(case) => cases::execute(
                self.adapter,
                self.host,
                &cases::concrete(self.adapter, case)?,
            ),
            Work::Generated(case) => cases::execute(
                self.adapter,
                self.host,
                &cases::generated(case, self.adapter)?,
            ),
            Work::Tree(case) => tree::execute(self.adapter, case),
        }
    }
}

/// The cases that run, in file-name order, beside the cases left out and why.
pub struct Plan<'a> {
    /// The cases that run.
    pub cases: Vec<Planned<'a>>,
    /// The cases left out.
    pub skipped: Vec<Skipped>,
}

/// What a run made of each case.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct RunResult {
    /// The cases that passed, those with their reports unobserved among them.
    pub passed: Vec<Identity>,
    /// The cases that failed, each with its failure.
    pub failed: Vec<(Identity, Failure)>,
    /// The cases left out.
    pub skipped: Vec<Skipped>,
    /// The cases that passed with their reports unobserved.
    pub unobserved: Vec<Identity>,
}

const LEVELS: [&str; 3] = ["core", "intl", "extensions"];

fn refuse(message: impl Into<String>) -> RunError {
    RunError::new(message)
}

fn unknown_level(levels: &[String], place: &str) -> Result<(), RunError> {
    match levels
        .iter()
        .find(|level| !LEVELS.contains(&level.as_str()))
    {
        Some(level) => Err(refuse(format!(
            "{place} names the level {}; the levels are core, intl and extensions.",
            js::quote(level)
        ))),
        None => Ok(()),
    }
}

// The adapter's statements about itself, and the levels option, are held to
// the vocabulary of sections 2, 11.2, 11.5 and 13, and of CST.md section 4,
// before anything runs: a claim outside it is an error, not a skip.
fn claims(adapter: &dyn Adapter, options: &Options<'_>) -> Result<(), RunError> {
    let levels = adapter.levels();

    unknown_level(&levels, "The adapter")?;

    if !levels.iter().any(|level| level == "core") {
        return Err(refuse("The adapter must claim the core level."));
    }

    if let Some(chosen) = options.levels {
        let chosen: Vec<String> = chosen.iter().map(|level| (*level).to_owned()).collect();

        unknown_level(&chosen, "The levels option")?;

        if let Some(unclaimed) = chosen.iter().find(|level| !levels.contains(level)) {
            return Err(refuse(format!(
                "The adapter does not claim the {unclaimed} level."
            )));
        }
    }

    let limits = adapter.limits();
    let declared = [
        ("output", limits.output),
        ("read", limits.read),
        ("conversion", limits.conversion),
        ("nesting", limits.nesting),
    ];

    if let Some((name, _)) = declared.iter().find(|(_, limit)| *limit < 1) {
        return Err(refuse(format!(
            "The adapter must declare its {name} limit as a positive integer."
        )));
    }

    if adapter.reentry().is_some_and(|bound| bound < 0) {
        return Err(refuse(
            "The adapter must state its bound on counted reads, reentry, as a non-negative integer.",
        ));
    }

    if !matches!(adapter.resolutions(), Value::Undefined | Value::Bool(_)) {
        return Err(refuse(
            "The adapter must state whether its bound counts a resolution asked for inside another, resolutions, as true or false.",
        ));
    }

    for entry in adapter.unexpressible().iter().flatten() {
        if !Api::ALL.iter().any(|api| api.as_str() == entry.api) {
            return Err(refuse(format!(
                "The adapter names the facility {}; the facilities are {}.",
                js::quote(&entry.api),
                Api::ALL.map(Api::as_str).join(", ")
            )));
        }
    }

    if let Some(claim) = adapter.cst()
        && !UNITS.contains(&claim.unit.as_str())
    {
        return Err(refuse(format!(
            "The adapter counts its spans in {}; the units are {}.",
            js::quote(&claim.unit),
            UNITS.join(", ")
        )));
    }

    Ok(())
}

// Why a level's cases are left out, or nothing where they run.
fn exclusion(level: Level, claimed: &[String], options: &Options<'_>) -> Option<String> {
    let name = level.as_str();

    if !claimed.iter().any(|claim| claim == name) {
        return Some(format!("The adapter does not claim the {name} level."));
    }

    if options.levels.is_some_and(|levels| !levels.contains(&name)) {
        return Some(format!(
            "The {name} level is not among the levels being run."
        ));
    }

    None
}

// Whether a request's properties hold the given value under a name, compared
// as a request is compared: by its entries, and with the sign of zero.
fn holds(options: &serde_json::Map<String, serde_json::Value>, name: &str, value: &Value) -> bool {
    options
        .get(name)
        .is_some_and(|held| js::canonical(&plain(held)) == js::canonical(value))
}

// The skip reason an entry gives a request it covers, or nothing where it
// covers none: the property must be one the request reads, among the values
// the entry lists, in a request meeting its condition. An empty list is no
// narrowing.
pub(crate) fn covers(
    entry: &Unexpressible,
    api: &str,
    options: &serde_json::Map<String, serde_json::Value>,
) -> Option<String> {
    let held = options.get(&entry.property)?;

    if !entry.values.is_empty()
        && !entry
            .values
            .iter()
            .any(|value| holds(options, &entry.property, value))
    {
        return None;
    }

    if !entry
        .when
        .iter()
        .all(|(name, value)| holds(options, name, value))
    {
        return None;
    }

    let value = if entry.values.is_empty() {
        String::new()
    } else {
        format!("the value {} of ", js::canonical(&plain(held)))
    };
    let condition = if entry.when.is_empty() {
        String::new()
    } else {
        format!(
            " whose {}",
            entry
                .when
                .iter()
                .map(|(name, held)| format!("{name} is {}", js::canonical(held)))
                .collect::<Vec<_>>()
                .join(" and ")
        )
    };

    Some(format!(
        "The adapter cannot express {value}the {} property of a {api} request{condition}.",
        entry.property
    ))
}

// A case whose request reads what the adapter documented it cannot express
// (section 11.2) measures the host's facility rather than the implementation,
// so it is left out the way an unclaimed level is. The first entry that
// covers the request gives the reason.
pub(crate) fn unexpressible(case: &Case, declared: &[Unexpressible]) -> Option<String> {
    let Case::Concrete(case) = case else {
        return None;
    };
    let format = case.expected.format.as_ref()?;
    let api = format.api.as_str();
    let empty = serde_json::Map::new();
    let options = match &format.options {
        Some(options) => options.as_object()?,
        None => &empty,
    };

    declared
        .iter()
        .filter(|entry| entry.api == api && !entry.property.is_empty())
        .find_map(|entry| covers(entry, api, options))
}

// The behaviours that read a kept input only after the modifier that kept it
// returned: `keep` reads nothing it keeps, so only a later modifier does, and
// `first` reads only what an earlier one kept.
const LATER: [Keeping; 2] = [Keeping::Keep, Keeping::First];

const KEEPS: &str = "The adapter states that a modifier cannot keep an input past its call.";

// The behaviours that ask for a resolution inside another.
const NESTS: [Keeping; 3] = [Keeping::Nest, Keeping::KeepNest, Keeping::KeepNestNe];

const NESTED: &str = "The adapter states a bound of 0 on counted reads, so the resolution a modifier asks for inside another may be refused.";

// The construction of resolutions one inside another keeps no input, so what
// the adapter states of keeping does not reach it. It measures a bound that
// counts resolutions, and writes two brackets for each that stands, which the
// output limit has to hold.
fn counting(adapter: &dyn Adapter) -> Option<String> {
    let Some(bound) = adapter.reentry() else {
        return Some("The adapter states no bound on counted reads.".to_owned());
    };

    if !matches!(adapter.resolutions(), Value::Bool(true)) {
        return Some(
            "The adapter does not state that its bound counts a resolution asked for inside another."
                .to_owned(),
        );
    }

    let brackets = cases::renested(usize::try_from(bound).unwrap_or(usize::MAX)).len();

    (i64::try_from(brackets).unwrap_or(i64::MAX) > adapter.limits().output).then(|| {
        "The output limit the adapter states does not hold the brackets of as many resolutions as its bound lets stand.".to_owned()
    })
}

// A case built from the bound on counted reads, or one that reads a kept
// input after the modifier that kept it returned, measures what the adapter
// stated it does not do, and is left out the way an unclaimed level is. So is
// a resolution asked for inside another at a bound of 0, which an
// implementation whose bound counts resolutions refuses.
fn kept(case: &Case, adapter: &dyn Adapter) -> Option<String> {
    let reentry = adapter.reentry();

    match case {
        Case::Concrete(case) => {
            let registered: Vec<Keeping> = case
                .keeping
                .iter()
                .flatten()
                .map(|(_, behaviour)| *behaviour)
                .collect();

            if !adapter.keeps() && registered.iter().any(|behaviour| LATER.contains(behaviour)) {
                return Some(KEEPS.to_owned());
            }

            (reentry == Some(0) && registered.iter().any(|behaviour| NESTS.contains(behaviour)))
                .then(|| NESTED.to_owned())
        }
        Case::Generated(case) => {
            if case.generate == cases::RESOLUTIONS {
                return counting(adapter);
            }

            if !cases::COUNTED.contains(&case.generate.as_str()) {
                return None;
            }

            // Built from resolutions, the two built around a resolution a
            // modifier asks for read no kept input after its modifier returned.
            let framed = cases::FRAMED.contains(&case.generate.as_str());

            if !adapter.keeps() && !(framed && matches!(adapter.resolutions(), Value::Bool(true))) {
                return Some(KEEPS.to_owned());
            }

            match reentry {
                None => Some("The adapter states no bound on counted reads.".to_owned()),
                Some(0) if case.generate == "reentry-own-reads-free" => Some(
                    "The adapter states a bound of 0 on counted reads, so no counted read stands for own reads to be made inside.".to_owned(),
                ),
                Some(0) if framed => Some(NESTED.to_owned()),
                Some(_) => None,
            }
        }
    }
}

// The constructions at and one past the conversion limit count their nodes in
// members that hold the host's undefined. Where the adapter states no
// stand-in for it, those members are left out, and what remains is an object
// of empty objects far short of the limit, which pins none.
fn stand_in(case: &Case, adapter: &dyn Adapter) -> Option<String> {
    let Case::Generated(case) = case else {
        return None;
    };

    (!adapter.undefined()
        && ["conversion-at-limit", "conversion-over-limit"].contains(&case.generate.as_str()))
    .then(|| {
        "The adapter states no stand-in for undefined, which the construction counts its nodes in."
            .to_owned()
    })
}

// A `refusing` tag stands for a mapping whose entries raise where they are
// read. The runner hands an adapter plain data, whose entries never run code,
// so it has no such value, and a case whose inputs hold the tag is left out.
// Leaving the tag out instead would hand over values that refuse nothing.
fn refusing(case: &Case) -> Option<String> {
    let Case::Concrete(case) = case else {
        return None;
    };
    let inputs = [
        Some(&case.message),
        case.payload.as_ref(),
        case.props.as_ref(),
        case.message_id.as_ref(),
        case.defaults.as_ref(),
    ];

    inputs.into_iter().flatten().any(refuses).then(|| {
        "Its inputs hold a refusing tag, and the runner hands the adapter plain data, which holds no value whose entries raise where they are read.".to_owned()
    })
}

// A file whose format this runner does not read is refused rather than
// skipped: what a runner could not read is not what an implementation passed.
fn readable(set: &Set) -> Result<(), RunError> {
    match set.fixtures.iter().find_map(|fixture| match &fixture.file {
        FixtureFile::Unknown { format } => Some((fixture, format)),
        _ => None,
    }) {
        Some((fixture, format)) => Err(refuse(format!(
            "The fixture file {} targets the format {}; this set reads {FORMAT}.",
            fixture.name,
            js::quote(format)
        ))),
        None => Ok(()),
    }
}

/// One entry per case the adapter's claims require, each ready to run, beside
/// the cases left out and why. The claims are checked first, and a claim
/// outside the format's vocabulary refuses the run.
pub fn plan<'a>(adapter: &'a dyn Adapter, options: &Options<'a>) -> Result<Plan<'a>, RunError> {
    claims(adapter, options)?;

    let set = match options.set {
        Some(set) => set,
        None => vendored()?,
    };

    readable(set)?;

    let levels = adapter.levels();
    let declared = adapter.unexpressible().unwrap_or_default();
    let offers_tree = adapter.cst().is_some();
    let mut plan = Plan {
        cases: Vec::new(),
        skipped: Vec::new(),
    };
    let mut place = |identity: Identity, work: Work<'a>, skip: Option<String>| match skip {
        Some(reason) => plan.skipped.push(Skipped { identity, reason }),
        None => plan.cases.push(Planned {
            identity,
            adapter,
            host: options.host,
            work,
        }),
    };

    for Fixture { name, file } in &set.fixtures {
        match file {
            FixtureFile::Resolution {
                level,
                section,
                cases,
            } => {
                for case in cases {
                    let (id, description, own, work) = match case {
                        Case::Concrete(concrete) => (
                            &concrete.id,
                            &concrete.description,
                            &concrete.section,
                            Work::Concrete(concrete),
                        ),
                        Case::Generated(generated) => (
                            &generated.id,
                            &generated.description,
                            &generated.section,
                            Work::Generated(generated),
                        ),
                    };
                    let identity = Identity {
                        id: id.clone(),
                        file: name.clone(),
                        level: Some(*level),
                        document: None,
                        section: own.clone().unwrap_or_else(|| section.clone()),
                        description: description.clone(),
                    };
                    let skip = exclusion(*level, &levels, options)
                        .or_else(|| unexpressible(case, &declared))
                        .or_else(|| stand_in(case, adapter))
                        .or_else(|| refusing(case))
                        .or_else(|| kept(case, adapter));

                    place(identity, work, skip);
                }
            }
            FixtureFile::Tree { section, cases } => {
                for case in cases {
                    let identity = Identity {
                        id: case.id.clone(),
                        file: name.clone(),
                        level: None,
                        document: Some("CST.md"),
                        section: case.section.clone().unwrap_or_else(|| section.clone()),
                        description: case.description.clone(),
                    };
                    let skip = (!offers_tree)
                        .then(|| "The adapter offers no concrete syntax tree.".to_owned());

                    place(identity, Work::Tree(case), skip);
                }
            }
            FixtureFile::Unknown { .. } => {}
        }
    }

    Ok(plan)
}

/// Executes a plan and sorts the outcomes.
pub fn run(adapter: &dyn Adapter, options: &Options<'_>) -> Result<RunResult, RunError> {
    let Plan { cases, skipped } = plan(adapter, options)?;
    let mut result = RunResult {
        skipped,
        ..RunResult::default()
    };

    for planned in cases {
        match planned.execute()? {
            Outcome::Passed => result.passed.push(planned.identity),
            Outcome::PassedReportsUnobserved => {
                result.unobserved.push(planned.identity.clone());
                result.passed.push(planned.identity);
            }
            Outcome::Failed(failure) => result.failed.push((planned.identity, failure)),
        }
    }

    Ok(result)
}

// A failure line carries text derived from the payload, and an output case
// can be as long as the output limit, so what is shown is bounded.
const SHOWN: usize = 200;

fn bounded(text: &str) -> String {
    match text.char_indices().nth(SHOWN) {
        Some((at, _)) => format!("{}...", &text[..at]),
        None => text.to_owned(),
    }
}

// Where the sentence a case pins is written. The document is named only where
// it is not the specification.
fn place(identity: &Identity) -> String {
    match identity.document {
        Some(document) => format!("{document} section {}", identity.section),
        None => format!("section {}", identity.section),
    }
}

impl Failure {
    /// The line the runner writes for a failed case.
    pub fn line(&self, identity: &Identity) -> String {
        format!(
            "FAIL {} ({}): {} Expected {}, actual {}. {}",
            identity.id,
            place(identity),
            self.reason,
            bounded(&self.expected),
            bounded(&self.actual),
            identity.description
        )
    }
}

impl Skipped {
    /// The line the runner writes for a case left out.
    pub fn line(&self) -> String {
        format!(
            "SKIP {} ({}): {}",
            self.identity.id,
            place(&self.identity),
            self.reason
        )
    }
}

/// The text the runner writes: one line per failure and per case left out,
/// then the counts.
pub fn summarize(result: &RunResult) -> String {
    let counts = format!(
        "{} passed, {} failed, {} skipped",
        result.passed.len(),
        result.failed.len(),
        result.skipped.len()
    );
    let unobserved = if result.unobserved.is_empty() {
        String::new()
    } else {
        format!(
            ", {} passed with reports unobserved",
            result.unobserved.len()
        )
    };

    result
        .failed
        .iter()
        .map(|(identity, failure)| failure.line(identity))
        .chain(result.skipped.iter().map(Skipped::line))
        .chain(std::iter::once(format!("{counts}{unobserved}")))
        .collect::<Vec<_>>()
        .join("\n")
}

/// Runs the set, and answers `Err` where anything failed, listing every
/// failure.
pub fn check(adapter: &dyn Adapter, options: &Options<'_>) -> Result<(), RunError> {
    let result = run(adapter, options)?;

    if result.failed.is_empty() {
        return Ok(());
    }

    Err(RunError::new(format!(
        "{} of {} conformance cases failed:\n{}",
        result.failed.len(),
        result.passed.len() + result.failed.len(),
        result
            .failed
            .iter()
            .map(|(identity, failure)| failure.line(identity))
            .collect::<Vec<_>>()
            .join("\n")
    )))
}
