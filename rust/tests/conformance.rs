#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::arithmetic_side_effects
)]

// The conformance set, one trial per case it plans for this implementation,
// and the audit of the runner against it. Every case runs on the exposure
// reading, where a case stating a request is held to the request the parser
// made; with `intl`, every case runs a second time on the text reading, where
// such a case is held to the text Node made of the request.

#[path = "conformance/adapter.rs"]
mod adapter;

use std::process::ExitCode;

use curly_message_conformance::{
    AuditOutcome, Case, FixtureFile, Level, Options, Outcome, Planned, RunError, Skipped, audit,
    plan, vendored,
};
use libtest_mimic::{Arguments, Failed, Trial};

use adapter::{ParserAdapter, Reading};

static EXPOSURE: ParserAdapter = ParserAdapter::new(Reading::Exposure);
#[cfg(feature = "intl")]
static TEXT: ParserAdapter = ParserAdapter::new(Reading::Text);

// The defects an adapter without the Intl level gives the audit nothing to
// reach: both alter what a formatting request is, and no case that states one
// runs.
const UNREACHABLE_WITHOUT_INTL: &[&str] = &["formats-wrong", "unexpressible-declared"];

fn failed(error: &RunError) -> Failed {
    error.message().into()
}

fn name(prefix: &str, planned: &Planned<'_>) -> String {
    let identity = &planned.identity;
    let place = match identity.document {
        Some(document) => format!("{document} section {}", identity.section),
        None => format!("section {}", identity.section),
    };

    format!("{prefix}{} ({place})", identity.id)
}

fn cases(
    adapter: &'static ParserAdapter,
    options: Options<'static>,
    prefix: &str,
) -> Result<(Vec<Trial>, Vec<Skipped>), RunError> {
    let planned = plan(adapter, &options)?;
    let trials = planned
        .cases
        .into_iter()
        .map(|case| {
            Trial::test(name(prefix, &case), move || match case.execute() {
                Ok(Outcome::Passed | Outcome::PassedReportsUnobserved) => Ok(()),
                Ok(Outcome::Failed(failure)) => Err(failure.line(&case.identity).into()),
                Err(error) => Err(failed(&error)),
            })
        })
        .collect();

    Ok((trials, planned.skipped))
}

fn audited(
    reading: &'static str,
    adapter: &'static ParserAdapter,
    options: Options<'static>,
) -> Trial {
    Trial::test(format!("the audit on the {reading} reading"), move || {
        let unreachable: &[&str] = if cfg!(feature = "intl") {
            &[]
        } else {
            UNREACHABLE_WITHOUT_INTL
        };
        let audited = audit(adapter, &options).map_err(|error| failed(&error))?;
        let wrong: Vec<String> = audited
            .iter()
            .filter(|entry| {
                let expected = if unreachable.contains(&entry.entry.id.as_str()) {
                    AuditOutcome::Unreachable
                } else {
                    AuditOutcome::Caught
                };

                entry.outcome != expected
            })
            .map(|entry| format!("{} {}", entry.entry.id, entry.outcome.as_str()))
            .collect();

        if audited.len() != 26 || !wrong.is_empty() {
            return Err(format!(
                "{} defects audited, unexpected: {}",
                audited.len(),
                wrong.join(", ")
            )
            .into());
        }

        Ok(())
    })
}

// Without `intl` the parser claims no Intl level, so the cases left out are
// exactly the Intl level's; with it, none is.
fn left_out(skipped: Vec<Skipped>) -> Trial {
    Trial::test(
        "the plan leaves out only what the claims leave out",
        move || {
            let set = vendored().map_err(|error| failed(&error))?;
            let intl: Vec<&str> = set
                .fixtures
                .iter()
                .filter_map(|fixture| match &fixture.file {
                    FixtureFile::Resolution {
                        level: Level::Intl,
                        cases,
                        ..
                    } => Some(cases),
                    _ => None,
                })
                .flatten()
                .map(|case| match case {
                    Case::Concrete(concrete) => concrete.id.as_str(),
                    Case::Generated(generated) => generated.id.as_str(),
                })
                .collect();
            let expected: Vec<(&str, &str)> = if cfg!(feature = "intl") {
                Vec::new()
            } else {
                intl.iter()
                    .map(|id| (*id, "The adapter does not claim the intl level."))
                    .collect()
            };
            let actual: Vec<(&str, &str)> = skipped
                .iter()
                .map(|skip| (skip.identity.id.as_str(), skip.reason.as_str()))
                .collect();

            if actual != expected {
                return Err(format!(
                    "{} cases left out where {} were expected: {:?}",
                    actual.len(),
                    expected.len(),
                    actual
                )
                .into());
            }

            Ok(())
        },
    )
}

// A feature unified in from another member of the workspace would turn the
// leg that runs without `intl` into one that runs with it; CI names the leg it
// runs, and an unset variable is a local run that checks nothing.
fn expected_feature() -> Trial {
    let expected = std::env::var("CURLY_EXPECT_INTL").ok();
    let unset = expected.is_none();

    Trial::test(
        "the intl feature is what the run expects",
        move || match expected.as_deref() {
            None => Ok(()),
            Some(flag @ ("0" | "1")) if (flag == "1") == cfg!(feature = "intl") => Ok(()),
            Some(flag) => Err(format!(
                "CURLY_EXPECT_INTL is {flag:?}, and the intl feature is {}.",
                if cfg!(feature = "intl") { "on" } else { "off" }
            )
            .into()),
        },
    )
    .with_ignored_flag(unset)
}

// ICU4X against Node on every request of the set: the requests whose text
// differs are the adapter's TEXT_DIFFERENCES, no more and no fewer.
#[cfg(feature = "intl")]
fn parity() -> Trial {
    use curly_message_conformance::{GoldenHost, Recorded};
    use curly_message_parser::{Api, IcuIntl, Intl};

    Trial::test("ICU4X agrees with Node on the set's requests", || {
        let host = GoldenHost::recorded().map_err(|error| failed(&error))?;
        let intl = IcuIntl::new();
        let mut differ = Vec::new();

        for (name, locale, stated, recorded) in host.requests() {
            let api = stated.api.as_str();
            let kept = adapter::request(api, &stated.options, &stated.input)
                .ok_or_else(|| format!("The request of {name} does not map."))?;
            let request = kept.request();
            let locale = locale.ok_or_else(|| format!("The request of {name} names no locale."))?;
            let answer = if request.api == Api::PluralRules {
                intl.select(locale, &request)
                    .map(|category| Recorded::Category(category.as_str().to_owned()))
            } else {
                intl.format(locale, &request).map(Recorded::Text)
            };
            let agrees = match (&answer, recorded) {
                (Err(_), Recorded::Error(_)) => true,
                (Ok(made), recorded) => made == recorded,
                (Err(_), _) => false,
            };

            if !agrees {
                differ.push((
                    locale.to_owned(),
                    adapter::request_text(api, &stated.options, &stated.input),
                ));
            }
        }

        differ.sort();
        differ.dedup();

        let mut listed: Vec<(String, String)> = adapter::TEXT_DIFFERENCES
            .iter()
            .map(|(locale, request, _)| ((*locale).to_owned(), (*request).to_owned()))
            .collect();

        listed.sort();

        if differ != listed {
            return Err(format!("The requests that differ are {differ:?}.").into());
        }

        Ok(())
    })
}

fn trials() -> Result<Vec<Trial>, RunError> {
    let (mut trials, skipped) = cases(&EXPOSURE, Options::default(), "")?;

    trials.push(left_out(skipped));
    trials.push(audited("exposure", &EXPOSURE, Options::default()));
    trials.push(expected_feature());

    #[cfg(feature = "intl")]
    {
        let text = Options {
            host: Some(curly_message_conformance::GoldenHost::recorded()?),
            ..Options::default()
        };
        let (on_text, _) = cases(&TEXT, text, "text reading: ")?;

        trials.extend(on_text);
        trials.push(audited("text", &TEXT, text));
        trials.push(parity());
    }

    Ok(trials)
}

fn main() -> ExitCode {
    let arguments = Arguments::from_args();
    let trials = trials()
        .unwrap_or_else(|error| vec![Trial::test("the set plans", move || Err(failed(&error)))]);

    libtest_mimic::run(&arguments, trials).exit_code()
}
