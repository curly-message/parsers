use std::sync::atomic::{AtomicBool, Ordering};

use crate::RunError;
use crate::adapter::{
    Adapter, CstClaim, ExposedRequest, Limits, Report, Resolution, Resolved, TreeAnswer,
    Unexpressible,
};
use crate::js;
use crate::plan::{Options, RunResult, run};
use crate::set::{DefectEntry, Verdict, vendored};
use crate::tree::{NAMED, boundaries, spelled};
use crate::value::Value;

/// A defect of the catalogue `defects.json`, as this runner carries it: each
/// wraps a conforming adapter into one that is wrong in exactly one way.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Defect {
    /// `output-truncated`.
    OutputTruncated,
    /// `output-trimmed`.
    OutputTrimmed,
    /// `answers-nothing`.
    AnswersNothing,
    /// `raises`.
    Raises,
    /// `reports-dropped`.
    ReportsDropped,
    /// `reports-extra`.
    ReportsExtra,
    /// `reports-reversed`.
    ReportsReversed,
    /// `report-code-changed`.
    ReportCodeChanged,
    /// `report-origin-changed`.
    ReportOriginChanged,
    /// `report-id-changed`.
    ReportIdChanged,
    /// `report-limit-changed`.
    ReportLimitChanged,
    /// `formats-wrong`.
    FormatsWrong,
    /// `reports-unobserved`.
    ReportsUnobserved,
    /// `claims-core-only`.
    ClaimsCoreOnly,
    /// `unexpressible-declared`.
    UnexpressibleDeclared,
    /// `claims-no-core`.
    ClaimsNoCore,
    /// `claims-unknown-level`.
    ClaimsUnknownLevel,
    /// `claims-no-limits`.
    ClaimsNoLimits,
    /// `tree-unoffered`.
    TreeUnoffered,
    /// `tree-node-dropped`.
    TreeNodeDropped,
    /// `tree-node-retyped`.
    TreeNodeRetyped,
    /// `tree-span-shifted`.
    TreeSpanShifted,
    /// `tree-name-raw`.
    TreeNameRaw,
    /// `tree-cancels-inverted`.
    TreeCancelsInverted,
    /// `tree-unit-changed`.
    TreeUnitChanged,
    /// `tree-unit-unknown`.
    TreeUnitUnknown,
}

impl Defect {
    /// Every defect this runner carries.
    pub const ALL: [Defect; 26] = [
        Defect::OutputTruncated,
        Defect::OutputTrimmed,
        Defect::AnswersNothing,
        Defect::Raises,
        Defect::ReportsDropped,
        Defect::ReportsExtra,
        Defect::ReportsReversed,
        Defect::ReportCodeChanged,
        Defect::ReportOriginChanged,
        Defect::ReportIdChanged,
        Defect::ReportLimitChanged,
        Defect::FormatsWrong,
        Defect::ReportsUnobserved,
        Defect::ClaimsCoreOnly,
        Defect::UnexpressibleDeclared,
        Defect::ClaimsNoCore,
        Defect::ClaimsUnknownLevel,
        Defect::ClaimsNoLimits,
        Defect::TreeUnoffered,
        Defect::TreeNodeDropped,
        Defect::TreeNodeRetyped,
        Defect::TreeSpanShifted,
        Defect::TreeNameRaw,
        Defect::TreeCancelsInverted,
        Defect::TreeUnitChanged,
        Defect::TreeUnitUnknown,
    ];

    /// The defect's id in the catalogue.
    pub fn id(self) -> &'static str {
        match self {
            Self::OutputTruncated => "output-truncated",
            Self::OutputTrimmed => "output-trimmed",
            Self::AnswersNothing => "answers-nothing",
            Self::Raises => "raises",
            Self::ReportsDropped => "reports-dropped",
            Self::ReportsExtra => "reports-extra",
            Self::ReportsReversed => "reports-reversed",
            Self::ReportCodeChanged => "report-code-changed",
            Self::ReportOriginChanged => "report-origin-changed",
            Self::ReportIdChanged => "report-id-changed",
            Self::ReportLimitChanged => "report-limit-changed",
            Self::FormatsWrong => "formats-wrong",
            Self::ReportsUnobserved => "reports-unobserved",
            Self::ClaimsCoreOnly => "claims-core-only",
            Self::UnexpressibleDeclared => "unexpressible-declared",
            Self::ClaimsNoCore => "claims-no-core",
            Self::ClaimsUnknownLevel => "claims-unknown-level",
            Self::ClaimsNoLimits => "claims-no-limits",
            Self::TreeUnoffered => "tree-unoffered",
            Self::TreeNodeDropped => "tree-node-dropped",
            Self::TreeNodeRetyped => "tree-node-retyped",
            Self::TreeSpanShifted => "tree-span-shifted",
            Self::TreeNameRaw => "tree-name-raw",
            Self::TreeCancelsInverted => "tree-cancels-inverted",
            Self::TreeUnitChanged => "tree-unit-changed",
            Self::TreeUnitUnknown => "tree-unit-unknown",
        }
    }

    /// The defect a catalogue id names, where this runner carries it.
    pub fn from_id(id: &str) -> Option<Self> {
        Self::ALL.into_iter().find(|defect| defect.id() == id)
    }

    /// The adapter wrapped so that it is wrong in this one way.
    pub fn wrap(self, adapter: &dyn Adapter) -> Mutated<'_> {
        Mutated {
            inner: adapter,
            defect: self,
            reached: AtomicBool::new(false),
        }
    }
}

/// A conforming adapter made wrong in one way, which remembers whether the
/// defect ever reached the runner.
pub struct Mutated<'a> {
    inner: &'a dyn Adapter,
    defect: Defect,
    reached: AtomicBool,
}

/// What the audit made of one defect.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum AuditOutcome {
    /// The runner answered what the catalogue expects.
    Caught,
    /// The runner answered something else.
    Missed,
    /// The defect never reached the runner: the adapter carried nothing it
    /// alters.
    Unreachable,
}

impl AuditOutcome {
    /// The outcome as the JavaScript runner spells it.
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Caught => "caught",
            Self::Missed => "missed",
            Self::Unreachable => "unreachable",
        }
    }
}

/// One defect of the catalogue, and what the runner made of it.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Audited {
    /// The catalogue's entry.
    pub entry: DefectEntry,
    /// What the runner answered: a verdict, or `None` where it answered
    /// nothing a defect shows as.
    pub observed: Option<Verdict>,
    /// Whether that is what the catalogue expects.
    pub outcome: AuditOutcome,
}

const OTHER_ID: &str = "an id no case names";

fn other_code(code: &str) -> &'static str {
    if code == "unknown-modifier" {
        "failed-modifier"
    } else {
        "unknown-modifier"
    }
}

fn other_origin(origin: &str) -> &'static str {
    if origin == "message" {
        "payload"
    } else {
        "message"
    }
}

// A request the set states nowhere, so a case compared on one fails on it.
fn other_request() -> ExposedRequest {
    ExposedRequest {
        api: "NumberFormat".to_owned(),
        options: Value::object(vec![("style".to_owned(), "percent".into())]),
        input: Value::Number(0.0),
        category: None,
    }
}

impl Mutated<'_> {
    /// Whether the defect reached the runner.
    pub fn reached(&self) -> bool {
        let inner = self.inner;

        match self.defect {
            Defect::AnswersNothing
            | Defect::Raises
            | Defect::ClaimsNoCore
            | Defect::ClaimsUnknownLevel
            | Defect::ClaimsNoLimits => true,
            Defect::FormatsWrong | Defect::UnexpressibleDeclared => {
                inner.levels().iter().any(|level| level == "intl")
            }
            Defect::ClaimsCoreOnly => inner.levels().len() > 1,
            Defect::TreeUnoffered | Defect::TreeUnitUnknown => inner.cst().is_some(),
            Defect::TreeUnitChanged => inner.cst().is_some_and(|claim| claim.unit != "code-point"),
            _ => self.reached.load(Ordering::Relaxed),
        }
    }

    fn mark(&self) {
        self.reached.store(true, Ordering::Relaxed);
    }

    // A defect of the reports, which an implementation need not observe at
    // all; `None` where these reports held nothing to alter.
    fn reports(&self, reports: &[Report]) -> Option<Vec<Report>> {
        match self.defect {
            Defect::ReportsDropped => (!reports.is_empty()).then(Vec::new),
            Defect::ReportsExtra => {
                let mut extra = reports.to_vec();

                extra.push(Report {
                    code: "unknown-modifier".to_owned(),
                    ..Report::default()
                });

                Some(extra)
            }
            Defect::ReportsReversed => {
                let reversed: Vec<_> = reports.iter().rev().cloned().collect();

                (reversed != reports).then_some(reversed)
            }
            Defect::ReportCodeChanged => (!reports.is_empty()).then(|| {
                reports
                    .iter()
                    .map(|report| Report {
                        code: other_code(&report.code).to_owned(),
                        ..report.clone()
                    })
                    .collect()
            }),
            Defect::ReportOriginChanged => reports
                .iter()
                .any(|report| report.origin.is_some())
                .then(|| {
                    reports
                        .iter()
                        .map(|report| Report {
                            origin: report
                                .origin
                                .as_deref()
                                .map(|origin| other_origin(origin).to_owned()),
                            ..report.clone()
                        })
                        .collect()
                }),
            Defect::ReportIdChanged => {
                reports.iter().any(|report| report.id.is_some()).then(|| {
                    reports
                        .iter()
                        .map(|report| Report {
                            id: report.id.as_ref().map(|_| OTHER_ID.to_owned()),
                            ..report.clone()
                        })
                        .collect()
                })
            }
            Defect::ReportLimitChanged => {
                reports
                    .iter()
                    .any(|report| report.limit.is_some())
                    .then(|| {
                        reports
                            .iter()
                            .map(|report| Report {
                                limit: report.limit.map(|limit| limit.saturating_add(1)),
                                ..report.clone()
                            })
                            .collect()
                    })
            }
            _ => None,
        }
    }

    // A defect that changes what the adapter answered; `None` where this
    // resolution held nothing to alter.
    fn answer(&self, resolved: &Resolved) -> Option<Resolved> {
        let output = &resolved.output;

        match self.defect {
            Defect::OutputTruncated => {
                let mut truncated = output.clone();

                truncated.pop()?;

                Some(Resolved {
                    output: truncated,
                    ..resolved.clone()
                })
            }
            Defect::OutputTrimmed => {
                let trimmed = js::trim(output);

                (trimmed != output).then(|| Resolved {
                    output: trimmed.to_owned(),
                    ..resolved.clone()
                })
            }
            Defect::ReportsUnobserved => resolved.reports.as_ref().map(|_| Resolved {
                reports: None,
                ..resolved.clone()
            }),
            Defect::FormatsWrong => Some(Resolved {
                formats: Some(vec![other_request()]),
                ..resolved.clone()
            }),
            _ => {
                let reports = self.reports(resolved.reports.as_ref()?)?;

                Some(Resolved {
                    reports: Some(reports),
                    ..resolved.clone()
                })
            }
        }
    }

    // A defect of the tree, applied to the reachable nodes it recognizes;
    // `None` where it recognized none.
    fn tree(&self, mut tree: TreeAnswer, message: &str) -> Option<TreeAnswer> {
        if self.defect == Defect::TreeNodeDropped {
            tree.nodes.first_mut()?.children.pop()?;

            return Some(tree);
        }

        let offsets = self
            .inner
            .cst()
            .map(|claim| boundaries(message, &claim.unit));
        let mut seen = vec![false; tree.nodes.len()];
        let mut stack = vec![0];
        let mut altered = false;

        while let Some(index) = stack.pop() {
            match seen.get_mut(index) {
                Some(seen) if !*seen => *seen = true,
                _ => continue,
            }

            let node = &mut tree.nodes[index];

            stack.extend(node.children.iter().rev());

            match self.defect {
                Defect::TreeNodeRetyped if node.kind == "separator" => {
                    node.kind = "text".to_owned();
                    altered = true;
                }
                Defect::TreeSpanShifted if node.kind == "placeholder" => {
                    node.start = node.start.saturating_add(1);
                    altered = true;
                }
                Defect::TreeCancelsInverted if node.kind == "escape" => {
                    node.cancels = Some(!node.cancels.unwrap_or(false));
                    altered = true;
                }
                Defect::TreeNameRaw if NAMED.contains(&node.kind.as_str()) => {
                    let raw = offsets
                        .as_deref()
                        .and_then(|offsets| spelled(message, offsets, node.start, node.end));

                    if let Some(raw) = raw
                        && node.name.as_deref() != Some(raw)
                    {
                        node.name = Some(raw.to_owned());
                        altered = true;
                    }
                }
                _ => {}
            }
        }

        altered.then_some(tree)
    }

    fn alters_tree(&self) -> bool {
        matches!(
            self.defect,
            Defect::TreeNodeDropped
                | Defect::TreeNodeRetyped
                | Defect::TreeSpanShifted
                | Defect::TreeNameRaw
                | Defect::TreeCancelsInverted
        )
    }
}

impl Adapter for Mutated<'_> {
    fn levels(&self) -> Vec<String> {
        let levels = self.inner.levels();

        match self.defect {
            Defect::ClaimsCoreOnly => vec!["core".to_owned()],
            Defect::ClaimsNoCore => levels.into_iter().filter(|level| level != "core").collect(),
            Defect::ClaimsUnknownLevel => {
                let mut levels = levels;

                levels.push("ecmascript".to_owned());
                levels
            }
            _ => levels,
        }
    }

    fn limits(&self) -> Limits {
        let limits = self.inner.limits();

        match self.defect {
            Defect::ClaimsNoLimits => Limits {
                nesting: 0,
                ..limits
            },
            _ => limits,
        }
    }

    fn unexpressible(&self) -> Option<Vec<Unexpressible>> {
        match self.defect {
            Defect::UnexpressibleDeclared => Some(vec![Unexpressible {
                api: "NumberFormat".to_owned(),
                property: "maximumFractionDigits".to_owned(),
                when: Vec::new(),
            }]),
            _ => self.inner.unexpressible(),
        }
    }

    fn cst(&self) -> Option<CstClaim> {
        let claim = self.inner.cst();

        match self.defect {
            Defect::TreeUnoffered => None,
            Defect::TreeUnitChanged => claim.map(|_| CstClaim {
                unit: "code-point".to_owned(),
            }),
            Defect::TreeUnitUnknown => claim.map(|_| CstClaim {
                unit: "utf-32".to_owned(),
            }),
            _ => claim,
        }
    }

    fn parse(&self, message: &str) -> Result<TreeAnswer, String> {
        let tree = self.inner.parse(message)?;

        if !self.alters_tree() {
            return Ok(tree);
        }

        match self.tree(tree.clone(), message) {
            Some(changed) => {
                self.mark();
                Ok(changed)
            }
            None => Ok(tree),
        }
    }

    fn resolve(&self, input: &Resolution) -> Result<Option<Resolved>, String> {
        match self.defect {
            Defect::AnswersNothing => return Ok(None),
            Defect::Raises => return Err("The raises defect raised.".to_owned()),
            _ => {}
        }

        let resolved = self.inner.resolve(input)?;
        let Some(resolved) = resolved else {
            return Ok(None);
        };

        match self.answer(&resolved) {
            Some(changed) => {
                self.mark();
                Ok(Some(changed))
            }
            None => Ok(Some(resolved)),
        }
    }
}

// What the runner answered for a defect, read against what it answered
// without one: a case that failed, a case left out, or a case that passed with
// its reports unchecked, where the run without the defect had none of those.
fn verdict(result: &RunResult, baseline: &RunResult) -> Option<Verdict> {
    if result.failed.len() > baseline.failed.len() {
        Some(Verdict::Fail)
    } else if result.skipped.len() > baseline.skipped.len() {
        Some(Verdict::Skip)
    } else if result.unobserved.len() > baseline.unobserved.len() {
        Some(Verdict::Unobserved)
    } else {
        None
    }
}

/// Runs the catalogue of defects against an adapter that passes the set, and
/// answers what this runner made of each: it caught it, it missed it, or the
/// defect never reached it. Each defect is run with the same options as the
/// baseline, so the audit measures whichever reading the adapter answers on.
pub fn audit(adapter: &dyn Adapter, options: &Options<'_>) -> Result<Vec<Audited>, RunError> {
    let baseline = run(adapter, options)?;

    if !baseline.failed.is_empty() {
        return Err(RunError::new(format!(
            "The audit needs an adapter that passes the set; this one fails {} of {} cases.",
            baseline.failed.len(),
            baseline.passed.len() + baseline.failed.len()
        )));
    }

    let catalogue = match options.set {
        Some(set) => &set.defects,
        None => &vendored()?.defects,
    };

    catalogue
        .iter()
        .map(|entry| {
            let defect = Defect::from_id(&entry.id).ok_or_else(|| {
                RunError::new(format!(
                    "The catalogue names a defect this runner does not carry: {}.",
                    js::quote(&entry.id)
                ))
            })?;
            let mutated = defect.wrap(adapter);
            let observed = match run(&mutated, options) {
                Ok(result) => verdict(&result, &baseline),
                Err(_) => Some(Verdict::Error),
            };
            let outcome = if !mutated.reached() {
                AuditOutcome::Unreachable
            } else if observed == Some(entry.expects) {
                AuditOutcome::Caught
            } else {
                AuditOutcome::Missed
            };

            Ok(Audited {
                entry: entry.clone(),
                observed,
                outcome,
            })
        })
        .collect()
}
