use std::sync::atomic::{AtomicBool, Ordering};

use crate::RunError;
use crate::adapter::{
    Adapter, CstClaim, ExposedRequest, Limits, Report, Resolution, Resolved, TreeAnswer,
    Unexpressible,
};
use crate::cases::{DEFINED, FRAMED, RESOLUTIONS, renested};
use crate::js;
use crate::plan::{Options, RunResult, covers, run};
use crate::set::{Case, DefectEntry, FixtureFile, Set, Verdict, vendored};
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
    /// `unexpressible-conditional-declared`.
    UnexpressibleConditionalDeclared,
    /// `claims-no-core`.
    ClaimsNoCore,
    /// `claims-unknown-level`.
    ClaimsUnknownLevel,
    /// `claims-no-limits`.
    ClaimsNoLimits,
    /// `conversion-limit-overstated`.
    ConversionLimitOverstated,
    /// `conversion-limit-understated`.
    ConversionLimitUnderstated,
    /// `reentry-limit-overstated`.
    ReentryLimitOverstated,
    /// `reentry-limit-understated`.
    ReentryLimitUnderstated,
    /// `claims-negative-reentry`.
    ClaimsNegativeReentry,
    /// `resolutions-overstated`.
    ResolutionsOverstated,
    /// `claims-non-boolean-resolutions`.
    ClaimsNonBooleanResolutions,
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
    pub const ALL: [Defect; 34] = [
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
        Defect::UnexpressibleConditionalDeclared,
        Defect::ClaimsNoCore,
        Defect::ClaimsUnknownLevel,
        Defect::ClaimsNoLimits,
        Defect::ConversionLimitOverstated,
        Defect::ConversionLimitUnderstated,
        Defect::ReentryLimitOverstated,
        Defect::ReentryLimitUnderstated,
        Defect::ClaimsNegativeReentry,
        Defect::ResolutionsOverstated,
        Defect::ClaimsNonBooleanResolutions,
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
            Self::UnexpressibleConditionalDeclared => "unexpressible-conditional-declared",
            Self::ClaimsNoCore => "claims-no-core",
            Self::ClaimsUnknownLevel => "claims-unknown-level",
            Self::ClaimsNoLimits => "claims-no-limits",
            Self::ConversionLimitOverstated => "conversion-limit-overstated",
            Self::ConversionLimitUnderstated => "conversion-limit-understated",
            Self::ReentryLimitOverstated => "reentry-limit-overstated",
            Self::ReentryLimitUnderstated => "reentry-limit-understated",
            Self::ClaimsNegativeReentry => "claims-negative-reentry",
            Self::ResolutionsOverstated => "resolutions-overstated",
            Self::ClaimsNonBooleanResolutions => "claims-non-boolean-resolutions",
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

    /// The adapter wrapped so that it is wrong in this one way, realized for
    /// a run of the vendored set at every level the adapter claims.
    pub fn wrap(self, adapter: &dyn Adapter) -> Mutated<'_> {
        self.realize(adapter, None, adapter.levels())
    }

    // The adapter wrapped for the run an audit makes, which may read another
    // set or fewer levels than the adapter claims: whether a defect of a
    // claim reaches the runner is read off the cases that run would hold.
    fn realize<'a>(
        self,
        adapter: &'a dyn Adapter,
        set: Option<&'a Set>,
        levels: Vec<String>,
    ) -> Mutated<'a> {
        Mutated {
            inner: adapter,
            defect: self,
            reached: AtomicBool::new(false),
            observes: AtomicBool::new(false),
            set,
            levels,
        }
    }
}

/// A conforming adapter made wrong in one way, which remembers whether the
/// defect ever reached the runner.
pub struct Mutated<'a> {
    inner: &'a dyn Adapter,
    defect: Defect,
    reached: AtomicBool,
    // Whether a resolution the run asked for answered reports.
    observes: AtomicBool,
    set: Option<&'a Set>,
    levels: Vec<String>,
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

// The entry each defect of what the adapter declares it cannot express adds to
// what the adapter declared itself, so the gaps it documents stay left out.
fn declared_entry(defect: Defect) -> Option<Unexpressible> {
    let property = "maximumFractionDigits".to_owned();
    let api = "NumberFormat".to_owned();

    match defect {
        Defect::UnexpressibleDeclared => Some(Unexpressible {
            api,
            property,
            values: Vec::new(),
            when: Vec::new(),
        }),
        Defect::UnexpressibleConditionalDeclared => Some(Unexpressible {
            api,
            property,
            values: vec![Value::Number(2.0)],
            when: vec![("useGrouping".to_owned(), Value::Bool(false))],
        }),
        _ => None,
    }
}

// The constructions a defect of a declared number is read by: only the
// construction at the conversion limit visits a node more than the limit the
// implementation imposes where the limit is stated one higher, and only those
// past it visit no node more where it is stated one lower. Every construction
// of kept reads that stands at the bound asks for one read more than the
// implementation lets stand where the bound is stated one higher, and only
// those past it let one through that the case expects refused where it is
// stated one lower. The two built around a resolution a modifier asks for are
// read as `Mutated::shows` says.
fn read_by(defect: Defect) -> &'static [&'static str] {
    match defect {
        Defect::ConversionLimitOverstated => &["conversion-at-limit"],
        Defect::ConversionLimitUnderstated => {
            &["conversion-over-limit", "conversion-over-limit-continues"]
        }
        Defect::ReentryLimitOverstated => &[
            "reentry-at-limit",
            "reentry-over-limit",
            "reentry-refused-not-recorded",
            "reentry-own-reads-free",
            "reentry-over-limit-by-a-conversion",
            "reentry-own-read-in-a-nested-resolution",
            DEFINED,
        ],
        Defect::ReentryLimitUnderstated => &[
            "reentry-over-limit",
            "reentry-refused-not-recorded",
            "reentry-over-limit-by-a-conversion",
            DEFINED,
        ],
        _ => &[],
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
            | Defect::ClaimsNoLimits
            | Defect::ClaimsNegativeReentry
            | Defect::ClaimsNonBooleanResolutions => true,
            Defect::FormatsWrong => inner.levels().iter().any(|level| level == "intl"),
            Defect::UnexpressibleDeclared | Defect::UnexpressibleConditionalDeclared => {
                self.leaves_out()
            }
            Defect::ConversionLimitOverstated | Defect::ConversionLimitUnderstated => {
                self.runs_any(&self.conversions())
            }
            Defect::ReentryLimitOverstated | Defect::ReentryLimitUnderstated => {
                self.bound().is_some()
                    && self.runs_where(|construction| {
                        read_by(self.defect).contains(&construction) && self.shows(construction)
                    })
            }
            // The construction of resolutions one inside another then runs
            // over an implementation that refuses none of them, and the one
            // whose read is refused beneath a modifier of the format is built
            // from resolutions that take no place, so above a bound of 1 its
            // read is let through. An adapter that states the claim itself
            // carries nothing to alter; the first is left out all the same
            // where it states no bound or its output limit does not hold it,
            // and the second shows nothing where it states none, 0 or 1.
            Defect::ResolutionsOverstated => {
                !matches!(inner.resolutions(), Value::Bool(true))
                    && inner.reentry().is_some_and(|bound| {
                        let brackets = renested(usize::try_from(bound).unwrap_or(usize::MAX)).len();
                        let held =
                            i64::try_from(brackets).unwrap_or(i64::MAX) <= inner.limits().output;

                        self.runs_where(|construction| {
                            (construction == RESOLUTIONS && held)
                                || (construction == DEFINED && bound > 1)
                        })
                    })
            }
            Defect::ClaimsCoreOnly => inner.levels().len() > 1,
            Defect::TreeUnoffered | Defect::TreeUnitUnknown | Defect::TreeUnitChanged => {
                inner.cst().is_some()
            }
            _ => self.reached.load(Ordering::Relaxed),
        }
    }

    fn run_set(&self) -> Option<&Set> {
        self.set.or_else(|| vendored().ok())
    }

    // The resolution cases of the run, at the levels it runs.
    fn cases(&self) -> impl Iterator<Item = &Case> {
        self.run_set()
            .into_iter()
            .flat_map(|set| &set.fixtures)
            .filter_map(|fixture| match &fixture.file {
                FixtureFile::Resolution { level, cases, .. }
                    if self.levels.iter().any(|run| run == level.as_str()) =>
                {
                    Some(cases)
                }
                _ => None,
            })
            .flatten()
    }

    // Whether the run holds one of the stated constructions.
    fn runs_any(&self, constructions: &[&str]) -> bool {
        self.runs_where(|construction| constructions.contains(&construction))
    }

    // Whether the run holds a construction `wanted` takes.
    fn runs_where(&self, wanted: impl Fn(&str) -> bool) -> bool {
        self.cases()
            .any(|case| matches!(case, Case::Generated(case) if wanted(&case.generate)))
    }

    // Whether a construction of kept reads shows a defect of the bound
    // stated. Those built from kept reads run where the adapter states that
    // a modifier can keep an input. The two built inside a resolution a
    // modifier asks for run at a bound above 0, and from resolutions,
    // whatever the adapter states of keeping, where it states that its bound
    // counts them. The one whose read is refused shows a bound stated one
    // lower however it is built, by a read let through, and one stated one
    // higher only built from resolutions: built from kept reads it stands a
    // place short of the bound where the bound counts reads alone, which the
    // adapter need not say. The other answers the same text where the
    // resolution is refused, and shows only a bound stated one higher, built
    // from resolutions, in the report of the modifier that asked, which
    // reaches the runner where that modifier sits in the resolution the case
    // asks for, at a stated bound of 1, and the adapter observes reports.
    fn shows(&self, construction: &str) -> bool {
        if !FRAMED.contains(&construction) {
            return self.inner.keeps();
        }

        let counts = matches!(self.inner.resolutions(), Value::Bool(true));
        let higher = self.defect == Defect::ReentryLimitOverstated;
        let declared = self.bound().unwrap_or(0);

        declared > 0
            && if construction == DEFINED {
                counts || (!higher && self.inner.keeps())
            } else {
                counts && higher && declared == 1 && self.observes.load(Ordering::Relaxed)
            }
    }

    // The constructions a defect of the conversion limit is read by, of
    // those the adapter runs: one that states no stand-in for `undefined`
    // runs none at or one past the limit, only the one that continues.
    fn conversions(&self) -> Vec<&'static str> {
        read_by(self.defect)
            .iter()
            .copied()
            .filter(|name| self.inner.undefined() || *name == "conversion-over-limit-continues")
            .collect()
    }

    // The bound a defect of the bound on counted reads states, where the
    // adapter states one and the defect leaves it a count of 0 or more.
    fn bound(&self) -> Option<i64> {
        let by = match self.defect {
            Defect::ReentryLimitOverstated => 1,
            Defect::ReentryLimitUnderstated => -1,
            _ => return None,
        };

        self.inner
            .reentry()
            .map(|bound| bound.saturating_add(by))
            .filter(|bound| *bound >= 0)
    }

    // Whether a case of the run states a request the defect's entry covers
    // and no entry of the adapter's own covers: a case only a runner that
    // reads the entry leaves out.
    fn leaves_out(&self) -> bool {
        let Some(entry) = declared_entry(self.defect) else {
            return false;
        };
        let own: Vec<Unexpressible> = self
            .inner
            .unexpressible()
            .unwrap_or_default()
            .into_iter()
            .filter(|declared| declared.api == entry.api && !declared.property.is_empty())
            .collect();
        let empty = serde_json::Map::new();

        self.cases().any(|case| {
            let Case::Concrete(case) = case else {
                return false;
            };
            let Some(format) = case
                .expected
                .format
                .as_ref()
                .filter(|format| format.api.as_str() == entry.api)
            else {
                return false;
            };
            let options = match &format.options {
                Some(options) => match options.as_object() {
                    Some(options) => options,
                    None => return false,
                },
                None => &empty,
            };

            covers(&entry, &entry.api, options).is_some()
                && !own
                    .iter()
                    .any(|declared| covers(declared, &entry.api, options).is_some())
        })
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
            Defect::ConversionLimitOverstated => Limits {
                conversion: limits.conversion.saturating_add(1),
                ..limits
            },
            Defect::ConversionLimitUnderstated => Limits {
                conversion: limits.conversion.saturating_sub(1),
                ..limits
            },
            _ => limits,
        }
    }

    fn unexpressible(&self) -> Option<Vec<Unexpressible>> {
        let own = self.inner.unexpressible();

        match declared_entry(self.defect) {
            Some(entry) => {
                let mut declared = own.unwrap_or_default();

                declared.push(entry);

                Some(declared)
            }
            None => own,
        }
    }

    fn undefined(&self) -> bool {
        self.inner.undefined()
    }

    fn reentry(&self) -> Option<i64> {
        match self.defect {
            Defect::ClaimsNegativeReentry => Some(-1),
            Defect::ReentryLimitOverstated | Defect::ReentryLimitUnderstated => {
                self.bound().or_else(|| self.inner.reentry())
            }
            _ => self.inner.reentry(),
        }
    }

    fn keeps(&self) -> bool {
        self.inner.keeps()
    }

    fn resolutions(&self) -> Value {
        match self.defect {
            Defect::ResolutionsOverstated => Value::Bool(true),
            Defect::ClaimsNonBooleanResolutions => "true".into(),
            _ => self.inner.resolutions(),
        }
    }

    fn cst(&self) -> Option<CstClaim> {
        let claim = self.inner.cst();

        match self.defect {
            Defect::TreeUnoffered => None,
            // A declaration of the adapter's own unit would alter nothing, so a
            // code-point adapter is misdeclared as UTF-16 and every other as
            // code points.
            Defect::TreeUnitChanged => claim.map(|claim| CstClaim {
                unit: if claim.unit == "code-point" {
                    "utf-16"
                } else {
                    "code-point"
                }
                .to_owned(),
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

        if resolved.reports.is_some() {
            self.observes.store(true, Ordering::Relaxed);
        }

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

    let levels: Vec<String> = adapter
        .levels()
        .into_iter()
        .filter(|level| {
            options
                .levels
                .is_none_or(|chosen| chosen.contains(&level.as_str()))
        })
        .collect();
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
            let mutated = defect.realize(adapter, options.set, levels.clone());
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
