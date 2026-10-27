#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::arithmetic_side_effects
)]

// The runner measured on a reference adapter over a toy implementation: a
// table of what each message of a hand-written mini set resolves to and how it
// is spelled, which passes that set on both readings. Each defect of the
// catalogue, wrapped around it, must then be caught.

use std::collections::HashMap;
use std::path::Path;
use std::sync::OnceLock;

use curly_message_conformance::{
    Adapter, Answer, AuditOutcome, Behaviour, BehaviourInput, Case, CstClaim, Defect, ExpectedNode,
    ExposedRequest, FixtureFile, FormatHost, FormatRequest, GoldenHost, Level, Limits, Options,
    RNode, Report, Resolution, Resolved, RunError, Set, TreeAnswer, Unexpressible, Value, Verdict,
    audit, behave, check, decode, load, plan, read_fixture, run, summarize, vendored,
};

fn mini() -> &'static Set {
    static SET: OnceLock<Set> = OnceLock::new();

    SET.get_or_init(|| load(&Path::new(env!("CARGO_MANIFEST_DIR")).join("tests/mini")).unwrap())
}

struct Entry {
    output: &'static str,
    reports: Vec<Report>,
    formats: Option<fn() -> ExposedRequest>,
    input: fn(&Resolution) -> bool,
}

fn report(code: &str, origin: &str, id: &str, limit: Option<i64>) -> Report {
    Report {
        code: code.to_owned(),
        origin: Some(origin.to_owned()),
        id: Some(id.to_owned()),
        limit,
    }
}

fn object(members: &[(&str, Value)]) -> Value {
    Value::object(
        members
            .iter()
            .map(|(name, value)| ((*name).to_owned(), value.clone()))
            .collect(),
    )
}

fn payload_text(input: &Resolution, name: &str) -> Option<String> {
    input.payload.get(name)?.as_str().map(str::to_owned)
}

fn any(_: &Resolution) -> bool {
    true
}

// The toy implementation: what each message resolves to.
fn table() -> HashMap<&'static str, Entry> {
    let limit = |code, origin, declared| vec![report(code, origin, "limits", declared)];

    HashMap::from([
        (
            "Hello",
            Entry {
                output: "Hello",
                reports: vec![],
                formats: None,
                input: any,
            },
        ),
        (
            " padded ",
            Entry {
                output: " padded ",
                reports: vec![],
                formats: None,
                input: any,
            },
        ),
        (
            "{{a:nope}}{{v}}",
            Entry {
                output: "",
                reports: vec![
                    report("unknown-modifier", "message", "mini.id", None),
                    report("unserializable-value", "payload", "mini.id", None),
                ],
                formats: None,
                input: |input| {
                    matches!(input.payload.get("v"), Some(Value::Unserializable))
                        && input.id.as_str() == Some("mini.id")
                },
            },
        ),
        (
            "{{v; a:xxx;}}",
            Entry {
                output: "xxx",
                reports: vec![],
                formats: None,
                input: |input| payload_text(input, "v").as_deref() == Some("a"),
            },
        ),
        (
            "A{{v; a:xxxx;}}B",
            Entry {
                output: "AB",
                reports: limit("output-limit", "limit", Some(3)),
                formats: None,
                input: any,
            },
        ),
        (
            "{{v; a:xxxx;}}{{w}}",
            Entry {
                output: "tail",
                reports: limit("output-limit", "limit", Some(3)),
                formats: None,
                input: |input| payload_text(input, "w").as_deref() == Some("tail"),
            },
        ),
        (
            "{{v:eq; nomatch:X; default:ok;}}",
            Entry {
                output: "ok",
                reports: vec![],
                formats: None,
                input: |input| payload_text(input, "v").as_deref() == Some("xxx"),
            },
        ),
        (
            "{{v:eq; nomatch:X; default:ok;}}{{w}}",
            Entry {
                output: "ok",
                reports: limit("read-limit", "limit", Some(3)),
                formats: None,
                input: |input| payload_text(input, "v").as_deref() == Some("xxxx"),
            },
        ),
        (
            "{{v; default:D}}",
            Entry {
                output: "D",
                reports: limit("unserializable-value", "payload", None),
                formats: None,
                // Two levels of the doubling tree visit seven nodes, the
                // first count past a conversion limit of 2 that it reaches.
                input: |input| match input.payload.get("v") {
                    Some(Value::Array(outer)) => outer.len() == 2,
                    _ => false,
                },
            },
        ),
        (
            "{{v; a:{{v; a:settled; default:fallback;}};}}",
            Entry {
                output: "settled",
                reports: vec![],
                formats: None,
                input: any,
            },
        ),
        (
            "{{v; a:{{v; a:{{v; a:settled; default:fallback;}};}};}}",
            Entry {
                output: "fallback",
                reports: limit("nesting-limit", "message", Some(2)),
                formats: None,
                input: any,
            },
        ),
        (
            "{{n:number; maximumFractionDigits:2}}",
            Entry {
                output: "1,234.57",
                reports: vec![],
                formats: Some(|| ExposedRequest {
                    api: "NumberFormat".to_owned(),
                    options: object(&[("maximumFractionDigits", Value::Number(2.0))]),
                    input: Value::Number(1234.5678),
                    category: None,
                }),
                input: |input| input.locale.as_deref() == Some("en"),
            },
        ),
        (
            "{{n:currency; currency:USD; maximumFractionDigits:0}}",
            Entry {
                output: "$2",
                reports: vec![],
                // The properties in another order than the case writes them.
                formats: Some(|| ExposedRequest {
                    api: "NumberFormat".to_owned(),
                    options: object(&[
                        ("maximumFractionDigits", Value::Number(0.0)),
                        ("currency", "USD".into()),
                        ("style", "currency".into()),
                    ]),
                    input: Value::Number(2.0),
                    category: None,
                }),
                input: any,
            },
        ),
        (
            "{{n:plural; one:# file; other:# files}}",
            Entry {
                output: "# file",
                reports: vec![],
                formats: Some(|| ExposedRequest {
                    api: "PluralRules".to_owned(),
                    options: Value::Undefined,
                    input: Value::Number(1.0),
                    category: Some("one".to_owned()),
                }),
                input: any,
            },
        ),
        (
            "Hi {{name}}!",
            Entry {
                output: "Hi !",
                reports: vec![],
                formats: None,
                input: any,
            },
        ),
    ])
}

// The toy implementation's trees: the mini set's own nodes, with spans counted
// in bytes. The runner checks every property of CST.md section 5 on them, so a
// tree built this way passes only where the set's nodes tile their messages.
fn arena(message: &str, expected: &[ExpectedNode]) -> TreeAnswer {
    fn place(nodes: &mut Vec<RNode>, wanted: &ExpectedNode, start: usize) -> usize {
        let index = nodes.len();
        let end = start + wanted.text.len();

        nodes.push(RNode {
            kind: wanted.kind.clone(),
            start: start as i64,
            end: end as i64,
            children: Vec::new(),
            name: wanted.name.clone(),
            cancels: wanted.cancels,
        });

        let mut at = start;

        for child in wanted.nodes.iter().flatten() {
            let placed = place(nodes, child, at);

            nodes[index].children.push(placed);
            at += child.text.len();
        }

        index
    }

    let mut nodes = vec![RNode {
        kind: "message".to_owned(),
        start: 0,
        end: message.len() as i64,
        children: Vec::new(),
        name: None,
        cancels: None,
    }];
    let mut at = 0;

    for wanted in expected {
        let placed = place(&mut nodes, wanted, at);

        nodes[0].children.push(placed);
        at += wanted.text.len();
    }

    TreeAnswer { nodes }
}

fn trees(set: &Set) -> HashMap<String, TreeAnswer> {
    set.fixtures
        .iter()
        .filter_map(|fixture| match &fixture.file {
            FixtureFile::Tree { cases, .. } => Some(cases),
            _ => None,
        })
        .flatten()
        .map(|case| (case.message.clone(), arena(&case.message, &case.expected)))
        .collect()
}

// A property the toy states it cannot express: the facility, the property,
// and the values other properties hold where it holds.
type Declared = (
    &'static str,
    &'static str,
    &'static [(&'static str, &'static str)],
);

struct Toy {
    expose: bool,
    levels: &'static [&'static str],
    unexpressible: Vec<Declared>,
    table: HashMap<&'static str, Entry>,
    trees: HashMap<String, TreeAnswer>,
}

impl Toy {
    fn new(expose: bool) -> Self {
        Self {
            expose,
            levels: &["core", "intl"],
            unexpressible: Vec::new(),
            table: table(),
            trees: trees(mini()),
        }
    }
}

impl Adapter for Toy {
    fn levels(&self) -> Vec<String> {
        self.levels
            .iter()
            .map(|level| (*level).to_owned())
            .collect()
    }

    fn limits(&self) -> Limits {
        Limits {
            output: 3,
            read: 3,
            conversion: 2,
            nesting: 2,
        }
    }

    fn unexpressible(&self) -> Option<Vec<Unexpressible>> {
        Some(
            self.unexpressible
                .iter()
                .map(|(api, property, when)| Unexpressible {
                    api: (*api).to_owned(),
                    property: (*property).to_owned(),
                    when: when
                        .iter()
                        .map(|(key, value)| ((*key).to_owned(), (*value).into()))
                        .collect(),
                })
                .collect(),
        )
    }

    fn cst(&self) -> Option<CstClaim> {
        Some(CstClaim {
            unit: "utf-8".to_owned(),
        })
    }

    fn parse(&self, message: &str) -> Result<TreeAnswer, String> {
        self.trees
            .get(message)
            .cloned()
            .ok_or_else(|| format!("The toy has no tree of {message}."))
    }

    fn resolve(&self, input: &Resolution) -> Result<Option<Resolved>, String> {
        let message = input.message.as_str().ok_or("The toy reads text alone.")?;
        let entry = self
            .table
            .get(message)
            .ok_or_else(|| format!("The toy cannot resolve {message}."))?;

        if !(entry.input)(input) {
            return Ok(Some(Resolved::output("the toy was handed other inputs")));
        }

        Ok(Some(Resolved {
            output: entry.output.to_owned(),
            reports: Some(entry.reports.clone()),
            formats: if self.expose {
                Some(entry.formats.iter().map(|request| request()).collect())
            } else {
                None
            },
        }))
    }
}

// The runner's host for the text reading of the mini set.
struct MiniHost;

impl FormatHost for MiniHost {
    fn perform(
        &self,
        locale: Option<&str>,
        request: &FormatRequest,
    ) -> Option<Result<String, String>> {
        if locale != Some("en") {
            return None;
        }

        let input = match request.input {
            Value::Number(input) => input,
            _ => return None,
        };
        let style = request.options.get("style").and_then(Value::as_str);

        Some(Ok(match (request.api.as_str(), style, input) {
            ("NumberFormat", None, 1234.5678) => "1,234.57".to_owned(),
            ("NumberFormat", Some("currency"), 2.0) => "$2".to_owned(),
            ("PluralRules", None, 1.0) => "one".to_owned(),
            _ => return None,
        }))
    }
}

fn exposure() -> Options<'static> {
    Options {
        set: Some(mini()),
        ..Options::default()
    }
}

fn text() -> Options<'static> {
    Options {
        set: Some(mini()),
        host: Some(&MiniHost),
        ..Options::default()
    }
}

fn ids(cases: &[curly_message_conformance::Identity]) -> Vec<&str> {
    cases.iter().map(|identity| identity.id.as_str()).collect()
}

#[test]
fn the_toy_passes_the_mini_set_on_both_readings() {
    for (adapter, options) in [(Toy::new(true), exposure()), (Toy::new(false), text())] {
        let result = run(&adapter, &options).unwrap();

        assert_eq!(summarize(&result), "18 passed, 0 failed, 0 skipped");
        assert!(check(&adapter, &options).is_ok());
    }
}

#[test]
fn the_text_reading_compares_the_output_with_what_the_host_made() {
    struct Wrong(Toy);

    impl Adapter for Wrong {
        fn levels(&self) -> Vec<String> {
            self.0.levels()
        }

        fn limits(&self) -> Limits {
            self.0.limits()
        }

        fn resolve(&self, input: &Resolution) -> Result<Option<Resolved>, String> {
            let mut resolved = self.0.resolve(input)?;

            if let Some(resolved) = &mut resolved
                && resolved.output == "1,234.57"
            {
                resolved.output = "1234.57".to_owned();
            }

            Ok(resolved)
        }
    }

    let wrong = Wrong(Toy::new(false));
    let result = run(&wrong, &text()).unwrap();

    assert_eq!(result.failed.len(), 1);
    assert_eq!(
        result.failed[0].0.id,
        "mini/number",
        "{}",
        summarize(&result)
    );
    assert_eq!(result.failed[0].1.reason, "The output differs.");
    assert_eq!(result.failed[0].1.expected, r#""1,234.57""#);

    // On the exposure reading the request is what is measured, and the toy
    // states the right one.
    let exposed = Wrong(Toy::new(true));

    assert!(run(&exposed, &exposure()).unwrap().failed.is_empty());
}

#[test]
fn a_request_with_no_host_or_no_answer_fails_rather_than_passes() {
    let adapter = Toy::new(false);
    let result = run(&adapter, &exposure()).unwrap();

    assert_eq!(
        ids(&result
            .failed
            .iter()
            .map(|(identity, _)| identity.clone())
            .collect::<Vec<_>>()),
        ["mini/number", "mini/currency", "mini/plural"]
    );
    assert!(result.failed.iter().all(|(_, failure)| {
        failure.reason
            == "The adapter exposes no request, and the runner has no host to perform it on."
    }));

    struct Mute;

    impl FormatHost for Mute {
        fn perform(&self, _: Option<&str>, _: &FormatRequest) -> Option<Result<String, String>> {
            None
        }
    }

    let options = Options {
        host: Some(&Mute),
        ..exposure()
    };
    let result = run(&adapter, &options).unwrap();

    assert_eq!(result.failed.len(), 3);
    assert_eq!(
        result.failed[0].1.reason,
        "The runner's host holds no answer recorded for the request."
    );

    struct Raising;

    impl FormatHost for Raising {
        fn perform(&self, _: Option<&str>, _: &FormatRequest) -> Option<Result<String, String>> {
            Some(Err("RangeError".to_owned()))
        }
    }

    let options = Options {
        host: Some(&Raising),
        ..exposure()
    };

    assert!(
        run(&adapter, &options)
            .unwrap_err()
            .message()
            .starts_with("The runner's host raised performing")
    );
}

#[test]
fn an_exposed_request_is_compared_by_its_entries_and_its_count() {
    struct Twice(Toy);

    impl Adapter for Twice {
        fn levels(&self) -> Vec<String> {
            self.0.levels()
        }

        fn limits(&self) -> Limits {
            self.0.limits()
        }

        fn resolve(&self, input: &Resolution) -> Result<Option<Resolved>, String> {
            let mut resolved = self.0.resolve(input)?;

            if let Some(Resolved {
                formats: Some(formats),
                ..
            }) = &mut resolved
            {
                match formats.first().map(|request| request.api.as_str()) {
                    Some("PluralRules") => formats[0].category = Some("few".to_owned()),
                    Some(_) if matches!(formats[0].input, Value::Number(2.0)) => {
                        formats[0].options = object(&[("style", "currency".into())]);
                    }
                    Some(_) => formats.push(formats[0].clone()),
                    None => {}
                }
            }

            Ok(resolved)
        }
    }

    let result = run(&Twice(Toy::new(true)), &exposure()).unwrap();
    let reasons: Vec<_> = result
        .failed
        .iter()
        .map(|(identity, failure)| (identity.id.as_str(), failure.reason.as_str()))
        .collect();

    assert_eq!(
        reasons,
        [
            ("mini/number", "Expected one formatting request, got 2."),
            ("mini/currency", "The properties of the request differ."),
            (
                "mini/plural",
                "The output differs from the one the category few selects."
            ),
        ]
    );
}

#[test]
fn every_defect_is_caught_against_the_toy_on_both_readings() {
    for (adapter, options) in [(Toy::new(true), exposure()), (Toy::new(false), text())] {
        let audited = audit(&adapter, &options).unwrap();

        assert_eq!(audited.len(), 26);

        for entry in &audited {
            assert_eq!(
                entry.outcome,
                AuditOutcome::Caught,
                "{} observed {:?}, expects {:?}",
                entry.entry.id,
                entry.observed,
                entry.entry.expects
            );
        }
    }
}

#[test]
fn a_defect_the_adapter_carries_nothing_for_is_unreachable() {
    struct Plain(Toy);

    impl Adapter for Plain {
        fn levels(&self) -> Vec<String> {
            vec!["core".to_owned()]
        }

        fn limits(&self) -> Limits {
            self.0.limits()
        }

        fn resolve(&self, input: &Resolution) -> Result<Option<Resolved>, String> {
            self.0.resolve(input)
        }
    }

    let audited = audit(&Plain(Toy::new(true)), &exposure()).unwrap();
    let unreachable: Vec<_> = audited
        .iter()
        .filter(|entry| entry.outcome == AuditOutcome::Unreachable)
        .map(|entry| entry.entry.id.as_str())
        .collect();

    assert_eq!(
        unreachable,
        [
            "formats-wrong",
            "claims-core-only",
            "unexpressible-declared",
            "tree-unoffered",
            "tree-node-dropped",
            "tree-node-retyped",
            "tree-span-shifted",
            "tree-name-raw",
            "tree-cancels-inverted",
            "tree-unit-changed",
            "tree-unit-unknown",
        ]
    );
    assert!(
        audited
            .iter()
            .all(|entry| entry.outcome != AuditOutcome::Missed)
    );
}

#[test]
fn the_audit_needs_an_adapter_that_passes() {
    let failing = Defect::Raises.wrap(&Toy::new(true)).reached();

    assert!(failing);
    assert!(
        audit(&Defect::OutputTruncated.wrap(&Toy::new(true)), &exposure())
            .unwrap_err()
            .message()
            .starts_with(
                "The audit needs an adapter that passes the set; this one fails 12 of 18 cases."
            )
    );
}

#[test]
fn the_catalogue_is_the_one_this_runner_carries() {
    let catalogue = &vendored().unwrap().defects;

    assert_eq!(
        catalogue
            .iter()
            .map(|entry| entry.id.as_str())
            .collect::<Vec<_>>(),
        Defect::ALL.map(Defect::id)
    );
    assert!(
        catalogue
            .iter()
            .all(|entry| Defect::from_id(&entry.id).is_some())
    );
    assert_eq!(
        catalogue
            .iter()
            .filter(|entry| entry.expects == Verdict::Error)
            .count(),
        4
    );
}

const ALWAYS: &[(&str, &str)] = &[];

const CURRENCY: &[(&str, &str)] = &[("style", "currency")];

fn refusal(adapter: &dyn Adapter, options: &Options<'_>) -> String {
    match plan(adapter, options) {
        Ok(_) => panic!("the plan was not refused"),
        Err(error) => error.message().to_owned(),
    }
}

#[test]
fn a_claim_outside_the_vocabulary_refuses_the_run() {
    let mut toy = Toy::new(true);

    toy.levels = &["core", "ecmascript"];
    assert_eq!(
        refusal(&toy, &exposure()),
        r#"The adapter names the level "ecmascript"; the levels are core, intl and extensions."#
    );

    toy.levels = &["intl"];
    assert_eq!(
        refusal(&toy, &exposure()),
        "The adapter must claim the core level."
    );

    toy.levels = &["core"];
    assert_eq!(
        refusal(
            &toy,
            &Options {
                levels: Some(&["intl"]),
                ..exposure()
            }
        ),
        "The adapter does not claim the intl level."
    );
    assert_eq!(
        refusal(
            &toy,
            &Options {
                levels: Some(&["Core"]),
                ..exposure()
            }
        ),
        r#"The levels option names the level "Core"; the levels are core, intl and extensions."#
    );

    toy.levels = &["core", "intl"];
    toy.unexpressible = vec![("Collator", "usage", ALWAYS)];
    assert_eq!(
        refusal(&toy, &exposure()),
        r#"The adapter names the facility "Collator"; the facilities are NumberFormat, DateTimeFormat, RelativeTimeFormat, PluralRules."#
    );

    assert_eq!(
        refusal(&Defect::ClaimsNoLimits.wrap(&Toy::new(true)), &exposure()),
        "The adapter must declare its nesting limit as a positive integer."
    );
    assert_eq!(
        refusal(&Defect::TreeUnitUnknown.wrap(&Toy::new(true)), &exposure()),
        r#"The adapter counts its spans in "utf-32"; the units are utf-8, utf-16, code-point."#
    );
}

#[test]
fn a_file_of_another_format_refuses_the_run() {
    let fixture = read_fixture(
        "next.json",
        r#"{ "format": "curly-message-4", "whatever": [] }"#,
    )
    .unwrap();
    let set = Set {
        fixtures: vec![fixture],
        defects: Vec::new(),
    };

    assert_eq!(
        refusal(
            &Toy::new(true),
            &Options {
                set: Some(&set),
                ..Options::default()
            }
        ),
        r#"The fixture file next.json targets the format "curly-message-4"; this set reads curly-message-3."#
    );
    assert!(
        read_fixture(
            "bad.json",
            r#"{ "format": "curly-message-3", "level": "core", "section": "1", "cases": [], "extra": 1 }"#
        )
        .is_err()
    );
}

#[test]
fn every_case_left_out_is_listed_with_its_reason() {
    let mut toy = Toy::new(true);

    toy.unexpressible = vec![
        ("NumberFormat", "maximumFractionDigits", CURRENCY),
        ("PluralRules", "localeMatcher", ALWAYS),
    ];

    let only_core = Options {
        levels: Some(&["core"]),
        ..exposure()
    };
    let planned = plan(&toy, &only_core).unwrap();
    let skipped: Vec<_> = planned
        .skipped
        .iter()
        .map(|skipped| (skipped.identity.id.as_str(), skipped.reason.as_str()))
        .collect();

    assert_eq!(planned.cases.len(), 15);
    assert_eq!(
        skipped,
        [
            (
                "mini/number",
                "The intl level is not among the levels being run."
            ),
            (
                "mini/currency",
                "The intl level is not among the levels being run."
            ),
            (
                "mini/plural",
                "The intl level is not among the levels being run."
            ),
        ]
    );

    // An entry with a `when` leaves out only the request that holds it.
    let planned = plan(&toy, &exposure()).unwrap();

    assert_eq!(
        planned
            .skipped
            .iter()
            .map(|skipped| (skipped.identity.id.as_str(), skipped.reason.as_str()))
            .collect::<Vec<_>>(),
        [(
            "mini/currency",
            "The adapter cannot express the maximumFractionDigits property of a NumberFormat request."
        )]
    );

    toy.levels = &["core"];

    let unoffered = Defect::TreeUnoffered.wrap(&toy);
    let planned = plan(&unoffered, &exposure()).unwrap();
    let reasons: Vec<_> = planned
        .skipped
        .iter()
        .map(|skipped| skipped.reason.as_str())
        .collect();

    assert_eq!(
        reasons,
        [
            "The adapter does not claim the intl level.",
            "The adapter does not claim the intl level.",
            "The adapter does not claim the intl level.",
            "The adapter offers no concrete syntax tree.",
            "The adapter offers no concrete syntax tree.",
            "The adapter offers no concrete syntax tree.",
            "The adapter offers no concrete syntax tree.",
        ]
    );

    let summary = summarize(&run(&unoffered, &exposure()).unwrap());

    assert!(summary.contains(
        "SKIP mini/astral-escape (CST.md section 7): The adapter offers no concrete syntax tree."
    ));
    assert!(summary.contains("SKIP mini/plural (section 11.5): The adapter does not claim"));
    assert!(summary.ends_with("11 passed, 0 failed, 7 skipped"));
}

#[test]
fn a_selection_case_must_name_every_category() {
    let file = r#"{
        "format": "curly-message-3", "level": "intl", "section": "11.5",
        "cases": [{
            "id": "x/plural", "description": "d", "message": "{{n:plural}}", "payload": { "n": 1 },
            "locale": "en",
            "expected": {
                "format": { "api": "PluralRules", "input": 1 },
                "categories": { "zero": "a", "one": "b", "two": "c", "few": "d", "other": "f" }
            }
        }]
    }"#;
    let set = Set {
        fixtures: vec![read_fixture("x.json", file).unwrap()],
        defects: Vec::new(),
    };
    let options = Options {
        set: Some(&set),
        ..Options::default()
    };

    assert_eq!(
        run(&Toy::new(true), &options).unwrap_err().message(),
        r#"The case x/plural names no output for the category "many"."#
    );
}

#[test]
fn a_case_expecting_nothing_or_naming_no_construction_ends_the_run() {
    for (file, message) in [
        (
            r#"{ "format": "curly-message-3", "level": "core", "section": "8",
                 "cases": [{ "id": "x/none", "description": "d", "message": "a", "expected": { "reports": [] } }] }"#,
            "The case x/none expects neither an output nor a format.",
        ),
        (
            r#"{ "format": "curly-message-3", "level": "core", "section": "13",
                 "cases": [{ "id": "x/gen", "description": "d", "generate": "output-far-over-limit" }] }"#,
            r#"The case x/gen names no construction: "output-far-over-limit"."#,
        ),
    ] {
        let set = Set {
            fixtures: vec![read_fixture("x.json", file).unwrap()],
            defects: Vec::new(),
        };
        let options = Options {
            set: Some(&set),
            ..Options::default()
        };

        assert_eq!(
            run(&Toy::new(true), &options).unwrap_err().message(),
            message
        );
    }
}

#[test]
fn a_raise_or_a_panic_fails_its_case_and_the_run_goes_on() {
    struct Panicking(Toy);

    impl Adapter for Panicking {
        fn levels(&self) -> Vec<String> {
            self.0.levels()
        }

        fn limits(&self) -> Limits {
            self.0.limits()
        }

        fn resolve(&self, input: &Resolution) -> Result<Option<Resolved>, String> {
            match input.message.as_str() {
                Some("Hello") => panic!("the toy panicked"),
                Some(" padded ") => Err("the toy raised".to_owned()),
                _ => self.0.resolve(input),
            }
        }
    }

    let result = run(&Panicking(Toy::new(true)), &exposure()).unwrap();
    let failed: Vec<_> = result
        .failed
        .iter()
        .map(|(identity, failure)| {
            (
                identity.id.as_str(),
                failure.reason.as_str(),
                failure.actual.as_str(),
            )
        })
        .collect();

    assert_eq!(
        failed,
        [
            ("mini/text", "The adapter raised.", r#""the toy panicked""#),
            ("mini/padded", "The adapter raised.", r#""the toy raised""#),
        ]
    );
    // The four tree cases are left out: this adapter offers no tree.
    assert_eq!(result.passed.len(), 12);
}

#[test]
fn a_tree_that_holds_itself_or_points_nowhere_fails_its_case() {
    struct Cyclic(Toy, usize);

    impl Adapter for Cyclic {
        fn levels(&self) -> Vec<String> {
            self.0.levels()
        }

        fn limits(&self) -> Limits {
            self.0.limits()
        }

        fn cst(&self) -> Option<CstClaim> {
            self.0.cst()
        }

        fn parse(&self, message: &str) -> Result<TreeAnswer, String> {
            let mut tree = self.0.parse(message)?;

            tree.nodes[0].children.push(self.1);
            tree.nodes[0].end = message.len() as i64;

            Ok(tree)
        }

        fn resolve(&self, input: &Resolution) -> Result<Option<Resolved>, String> {
            self.0.resolve(input)
        }
    }

    for (child, reason) in [
        (0, "The tree holds more nodes than the message can spell."),
        (1000, "The tree holds something that is not a node."),
    ] {
        let result = run(&Cyclic(Toy::new(true), child), &exposure()).unwrap();

        assert_eq!(result.failed.len(), 4, "{}", summarize(&result));
        assert!(
            result
                .failed
                .iter()
                .all(|(_, failure)| failure.reason == reason)
        );
    }
}

#[test]
fn a_failure_line_is_bounded() {
    struct Long(Toy);

    impl Adapter for Long {
        fn levels(&self) -> Vec<String> {
            self.0.levels()
        }

        fn limits(&self) -> Limits {
            self.0.limits()
        }

        fn resolve(&self, input: &Resolution) -> Result<Option<Resolved>, String> {
            match input.message.as_str() {
                Some("Hello") => Ok(Some(Resolved::output("\u{e9}".repeat(500)))),
                _ => self.0.resolve(input),
            }
        }
    }

    let summary = summarize(&run(&Long(Toy::new(true)), &exposure()).unwrap());
    let line = summary.lines().next().unwrap();

    assert_eq!(
        line,
        format!(
            "FAIL mini/text (section 8): The output differs. Expected \"Hello\", actual \"{}.... Text renders as written.",
            "\u{e9}".repeat(199)
        )
    );
}

#[test]
fn reports_are_compared_in_order_and_by_the_fields_both_sides_carry() {
    struct Bare(Toy);

    impl Adapter for Bare {
        fn levels(&self) -> Vec<String> {
            self.0.levels()
        }

        fn limits(&self) -> Limits {
            self.0.limits()
        }

        fn resolve(&self, input: &Resolution) -> Result<Option<Resolved>, String> {
            let mut resolved = self.0.resolve(input)?;

            for report in resolved
                .iter_mut()
                .flat_map(|resolved| resolved.reports.iter_mut().flatten())
            {
                report.origin = None;
                report.id = None;
                report.limit = None;
            }

            Ok(resolved)
        }
    }

    let result = run(&Bare(Toy::new(true)), &exposure()).unwrap();

    assert!(result.failed.is_empty(), "{}", summarize(&result));
    assert!(result.unobserved.is_empty());

    let unobserved = run(
        &Defect::ReportsUnobserved.wrap(&Toy::new(true)),
        &exposure(),
    )
    .unwrap();

    assert_eq!(unobserved.unobserved.len(), 14);
    assert!(
        summarize(&unobserved)
            .ends_with("18 passed, 0 failed, 0 skipped, 14 passed with reports unobserved")
    );
}

#[test]
fn tags_are_decoded_at_every_depth() {
    let json = serde_json::json!({
        "a": [{ "$curly": "undefined" }, { "b": { "$curly": "unserializable" } }],
        "n": { "$curly": "nodes", "count": 7 },
        "plain": { "$curly2": "kept" }
    });
    let value = decode(&json).unwrap();
    let Some(Value::Array(a)) = value.get("a") else {
        panic!("no array");
    };

    assert!(a[0].is_undefined());
    assert!(matches!(a[1].get("b"), Some(Value::Unserializable)));
    assert!(matches!(
        value.get("plain").and_then(|plain| plain.get("$curly2")),
        Some(Value::String(_))
    ));

    let Some(Value::Array(tree)) = value.get("n") else {
        panic!("no tree");
    };
    let (Value::Array(left), Value::Array(right)) = (&tree[0], &tree[1]) else {
        panic!("not doubled");
    };

    assert!(std::rc::Rc::ptr_eq(left, right));
    assert!(matches!(&left[0], Value::Array(leaf) if leaf.is_empty()));

    for (json, message) in [
        (
            serde_json::json!({ "$curly": "date" }),
            r#"Unknown tag "date"."#,
        ),
        (
            serde_json::json!([{ "$curly": "nodes" }]),
            "A nodes tag needs a numeric count.",
        ),
    ] {
        assert_eq!(decode(&json).unwrap_err(), RunError::new(message));
    }
}

struct Input;

impl BehaviourInput for Input {
    fn value(&self) -> String {
        "Stra\u{df}e 1".to_owned()
    }

    fn options(&self) -> Vec<(String, String)> {
        vec![
            ("b".to_owned(), "2".to_owned()),
            ("a".to_owned(), "1".to_owned()),
        ]
    }

    fn props(&self) -> Vec<(String, Value)> {
        vec![
            ("\u{e9}".to_owned(), Value::Number(1e21)),
            ("b".to_owned(), Value::Undefined),
            ("B".to_owned(), object(&[("x", Value::Null)])),
        ]
    }

    fn locale(&self) -> Option<String> {
        None
    }

    fn default(&self) -> String {
        "D".to_owned()
    }
}

fn answered(behaviour: Behaviour) -> String {
    match behave(behaviour, &Input) {
        Answer::Text(text) => text,
        Answer::Nothing => "<nothing>".to_owned(),
        Answer::Raise(_) => "<raise>".to_owned(),
        Answer::Value(value) => curly_message_conformance::js::show(&value),
    }
}

#[test]
fn the_behaviours_answer_as_the_catalogue_states() {
    assert_eq!(answered(Behaviour::Upper), "STRA\u{df}E 1");
    assert_eq!(answered(Behaviour::Echo), "Stra\u{df}e 1");
    assert_eq!(answered(Behaviour::Empty), "");
    assert_eq!(answered(Behaviour::Nothing), "<nothing>");
    assert_eq!(answered(Behaviour::Raise), "<raise>");
    assert_eq!(answered(Behaviour::Default), "D");
    assert_eq!(answered(Behaviour::Options), "b=2,a=1");
    assert_eq!(
        answered(Behaviour::Props),
        "B={\"x\":null},b=undefined,\u{e9}=1e+21"
    );
    assert_eq!(answered(Behaviour::Locale), "none");
    assert_eq!(
        answered(Behaviour::Object),
        "{\"answer\":\"Stra\u{df}e 1\"}"
    );
}

// Release 4.1.0 holds 771 cases: 459 at the Core level, eight of them built
// from the adapter's limits, 53 at the Extensions level, 215 at the Intl
// level, and 44 that pin the tree.
#[test]
fn the_vendored_set_is_the_release() {
    let set = vendored().unwrap();
    let mut counts = HashMap::new();
    let mut generated = 0;

    for fixture in &set.fixtures {
        match &fixture.file {
            FixtureFile::Resolution { level, cases, .. } => {
                *counts.entry(level.as_str()).or_insert(0) += cases.len();
                generated += cases
                    .iter()
                    .filter(|case| matches!(case, Case::Generated(_)))
                    .count();
            }
            FixtureFile::Tree { cases, .. } => *counts.entry("tree").or_insert(0) += cases.len(),
            FixtureFile::Unknown { .. } => panic!("an unknown file"),
        }
    }

    assert_eq!(
        counts,
        HashMap::from([
            ("core", 459),
            ("extensions", 53),
            ("intl", 215),
            ("tree", 44)
        ])
    );
    assert_eq!(generated, 8);
    assert_eq!(set.fixtures.len(), 18);
    assert_eq!(set.defects.len(), 26);
    assert_eq!(Level::Core.as_str(), "core");
}

#[test]
fn every_request_of_the_set_has_its_recorded_answer() {
    let host = GoldenHost::recorded().unwrap();
    let set = vendored().unwrap();
    let mut names = Vec::new();

    for fixture in &set.fixtures {
        let FixtureFile::Resolution { cases, .. } = &fixture.file else {
            continue;
        };

        for case in cases {
            let Case::Concrete(case) = case else {
                continue;
            };
            let Some(format) = &case.expected.format else {
                continue;
            };
            let request = FormatRequest {
                api: format.api,
                options: format
                    .options
                    .as_ref()
                    .map_or(Value::Undefined, curly_message_conformance::plain),
                input: curly_message_conformance::plain(&format.input),
            };

            assert!(
                host.perform(case.locale.as_deref(), &request).is_some(),
                "{}",
                case.id
            );
            names.push(case.id.clone());
        }
    }

    let recorded: Vec<_> = host
        .requests()
        .into_iter()
        .map(|(name, ..)| name.to_owned())
        .collect();

    assert!(recorded.iter().all(|name| names.contains(name)));
    assert_eq!(names.len(), 154);
    assert_eq!(recorded.len(), 119);
    assert_eq!(
        (host.node(), host.icu(), host.cldr()),
        ("22.22.0", "77.1", "47.0")
    );
}
