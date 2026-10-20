use std::collections::BTreeMap;
use std::fs;
use std::io::{Cursor, Read};
use std::path::Path;
use std::sync::OnceLock;

use serde::de::{Deserialize, Deserializer};
use serde_json::{Map, Value as Json};
use sha2::{Digest, Sha256};

use crate::RunError;

/// The versioned identifier of the format this runner reads.
pub const FORMAT: &str = "curly-message-3";

/// The release of the set this runner carries.
pub const VERSION: &str = "4.1.0";

/// The sha256 of `conformance-4.1.0.zip`, as the release notes state it.
pub const DIGEST: &str = "9011c603dd2a87f563557c60f32a8216f99cb1935dfcf54199f309e31496fbfd";

const ARCHIVE: &[u8] = include_bytes!("../set/conformance-4.1.0.zip");

/// A conformance set: its fixture files, sorted by name, and the catalogue of
/// defects a runner is audited against.
#[derive(Debug)]
pub struct Set {
    /// The fixture files, sorted by name.
    pub fixtures: Vec<Fixture>,
    /// The catalogue of `defects.json`.
    pub defects: Vec<DefectEntry>,
}

/// A fixture file together with the name it is shipped under.
#[derive(Debug)]
pub struct Fixture {
    /// The file name.
    pub name: String,
    /// What it holds.
    pub file: FixtureFile,
}

/// A fixture file, by what it pins.
#[derive(Debug)]
pub enum FixtureFile {
    /// Cases that pin a resolution.
    Resolution {
        /// The level every case requires.
        level: Level,
        /// The section the file pins.
        section: String,
        /// The cases.
        cases: Vec<Case>,
    },
    /// Cases that pin the tree of `CST.md`.
    Tree {
        /// The section of `CST.md` the file pins.
        section: String,
        /// The cases.
        cases: Vec<TreeCase>,
    },
    /// A file targeting a format this runner does not read, which a plan
    /// refuses rather than skips.
    Unknown {
        /// The format it names.
        format: String,
    },
}

/// A conformance level of section 2.
#[derive(Clone, Copy, Debug, PartialEq, Eq, serde::Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum Level {
    /// `core`.
    Core,
    /// `intl`.
    Intl,
    /// `extensions`.
    Extensions,
}

impl Level {
    /// The level as section 2 spells it.
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Core => "core",
            Self::Intl => "intl",
            Self::Extensions => "extensions",
        }
    }
}

/// A case of a resolution file.
#[derive(Debug)]
pub enum Case {
    /// A case written out.
    Concrete(Box<ConcreteCase>),
    /// A case built from the adapter's limits.
    Generated(GeneratedCase),
}

/// A case written out, its inputs still JSON: tags are decoded when it runs.
#[derive(Debug)]
pub struct ConcreteCase {
    /// The case's id.
    pub id: String,
    /// What it pins.
    pub description: String,
    /// A heading more specific than the file's.
    pub section: Option<String>,
    /// The message.
    pub message: Json,
    /// The payload.
    pub payload: Option<Json>,
    /// The caller's props.
    pub props: Option<Json>,
    /// The locale.
    pub locale: Option<String>,
    /// The message's id.
    pub message_id: Option<Json>,
    /// Host-defined modifiers, by name, in the order the case writes them.
    pub modifiers: Option<Vec<(String, Behaviour)>>,
    /// The implementation-configured defaults.
    pub defaults: Option<Json>,
    /// What the resolution must produce.
    pub expected: Expected,
}

/// A case built from the adapter's limits.
#[derive(Debug)]
pub struct GeneratedCase {
    /// The case's id.
    pub id: String,
    /// What it pins.
    pub description: String,
    /// A heading more specific than the file's.
    pub section: Option<String>,
    /// The construction, as the file names it.
    pub generate: String,
}

/// What a written-out case expects.
#[derive(Debug, serde::Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Expected {
    /// The output.
    pub output: Option<String>,
    /// The request a locale-dependent case states instead of an output.
    pub format: Option<ExpectedFormat>,
    /// For a `PluralRules` request: the output each category selects.
    pub categories: Option<Categories>,
    /// The reports, in order; none where it is left out.
    pub reports: Option<Vec<ExpectedReport>>,
}

/// A request as a case states it.
#[derive(Debug, serde::Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ExpectedFormat {
    /// The facility.
    pub api: Api,
    /// The options it is constructed with.
    #[serde(default, deserialize_with = "present")]
    pub options: Option<Json>,
    /// The input.
    pub input: Json,
}

/// The facilities of section 11.2 and 11.5.
#[derive(Clone, Copy, Debug, PartialEq, Eq, serde::Deserialize)]
pub enum Api {
    /// `NumberFormat`.
    NumberFormat,
    /// `DateTimeFormat`.
    DateTimeFormat,
    /// `RelativeTimeFormat`.
    RelativeTimeFormat,
    /// `PluralRules`.
    PluralRules,
}

impl Api {
    /// Every facility.
    pub const ALL: [Api; 4] = [
        Api::NumberFormat,
        Api::DateTimeFormat,
        Api::RelativeTimeFormat,
        Api::PluralRules,
    ];

    /// The facility as ECMA-402 names it.
    pub fn as_str(self) -> &'static str {
        match self {
            Self::NumberFormat => "NumberFormat",
            Self::DateTimeFormat => "DateTimeFormat",
            Self::RelativeTimeFormat => "RelativeTimeFormat",
            Self::PluralRules => "PluralRules",
        }
    }
}

/// The output a selection case names for each category. A category it
/// leaves out is the set's defect, found when the case is prepared.
#[derive(Debug, serde::Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Categories {
    /// `zero`.
    pub zero: Option<String>,
    /// `one`.
    pub one: Option<String>,
    /// `two`.
    pub two: Option<String>,
    /// `few`.
    pub few: Option<String>,
    /// `many`.
    pub many: Option<String>,
    /// `other`.
    pub other: Option<String>,
}

/// The plural categories, in the order section 11.5 lists them.
pub const CATEGORIES: [&str; 6] = ["zero", "one", "two", "few", "many", "other"];

impl Categories {
    /// The output named for a category.
    pub fn get(&self, category: &str) -> Option<&str> {
        match category {
            "zero" => self.zero.as_deref(),
            "one" => self.one.as_deref(),
            "two" => self.two.as_deref(),
            "few" => self.few.as_deref(),
            "many" => self.many.as_deref(),
            "other" => self.other.as_deref(),
            _ => None,
        }
    }
}

/// A report as a case expects it.
#[derive(Clone, Debug, serde::Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ExpectedReport {
    /// The code.
    pub code: Code,
    /// The origin.
    pub origin: Origin,
    /// The id, as text.
    pub id: Option<String>,
}

/// A report code of section 14.3.
#[derive(Clone, Copy, Debug, PartialEq, Eq, serde::Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum Code {
    /// `unknown-modifier`.
    UnknownModifier,
    /// `failed-modifier`.
    FailedModifier,
    /// `missing-options`.
    MissingOptions,
    /// `unserializable-value`.
    UnserializableValue,
    /// `missing-locale`.
    MissingLocale,
    /// `nesting-limit`.
    NestingLimit,
    /// `output-limit`.
    OutputLimit,
    /// `read-limit`.
    ReadLimit,
}

impl Code {
    /// The code as section 14.3 spells it.
    pub fn as_str(self) -> &'static str {
        match self {
            Self::UnknownModifier => "unknown-modifier",
            Self::FailedModifier => "failed-modifier",
            Self::MissingOptions => "missing-options",
            Self::UnserializableValue => "unserializable-value",
            Self::MissingLocale => "missing-locale",
            Self::NestingLimit => "nesting-limit",
            Self::OutputLimit => "output-limit",
            Self::ReadLimit => "read-limit",
        }
    }
}

/// A report origin of section 14.2.
#[derive(Clone, Copy, Debug, PartialEq, Eq, serde::Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum Origin {
    /// `message`.
    Message,
    /// `payload`.
    Payload,
    /// `limit`.
    Limit,
}

impl Origin {
    /// The origin as section 14.2 spells it.
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Message => "message",
            Self::Payload => "payload",
            Self::Limit => "limit",
        }
    }
}

/// A behaviour of the catalogue of host-defined modifiers.
#[derive(Clone, Copy, Debug, PartialEq, Eq, serde::Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum Behaviour {
    /// The value with its ASCII letters uppercased.
    Upper,
    /// The value.
    Echo,
    /// The empty string.
    Empty,
    /// No answer.
    Nothing,
    /// A raise.
    Raise,
    /// The default, read through the chain.
    Default,
    /// The options, each `key=value`, joined by commas.
    Options,
    /// The props, each `name=value` with the value as JSON, sorted by name.
    Props,
    /// The locale, or `none`.
    Locale,
    /// A plain object holding the value under `answer`.
    Object,
}

/// A case that puts a message to the implementation's tree.
#[derive(Debug, serde::Deserialize)]
#[serde(deny_unknown_fields)]
pub struct TreeCase {
    /// The case's id.
    pub id: String,
    /// What it pins.
    pub description: String,
    /// A heading of `CST.md` more specific than the file's.
    pub section: Option<String>,
    /// The message.
    pub message: String,
    /// The children of the root.
    pub expected: Vec<ExpectedNode>,
    /// What the message resolves to over no payload.
    pub resolves: Option<String>,
}

/// What a tree case expects of one node.
#[derive(Debug, serde::Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ExpectedNode {
    /// The kind.
    #[serde(rename = "type")]
    pub kind: String,
    /// The text the node spans.
    pub text: String,
    /// For a name: the span unescaped.
    pub name: Option<String>,
    /// For an escape: which reading it takes.
    pub cancels: Option<bool>,
    /// The children, where the case reads into the node.
    pub nodes: Option<Vec<ExpectedNode>>,
}

/// One defect of the catalogue.
#[derive(Clone, Debug, PartialEq, Eq, serde::Deserialize)]
#[serde(deny_unknown_fields)]
pub struct DefectEntry {
    /// Its id.
    pub id: String,
    /// What it does.
    pub description: String,
    /// The document `section` is a heading of, where it is not `SPEC.md`.
    pub document: Option<String>,
    /// The section it pins.
    pub section: Option<String>,
    /// What a correct runner answers where it is present.
    pub expects: Verdict,
}

/// What a runner answers for a defect.
#[derive(Clone, Copy, Debug, PartialEq, Eq, serde::Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum Verdict {
    /// The run is refused.
    Error,
    /// A case fails.
    Fail,
    /// A case is left out.
    Skip,
    /// A case passes with its reports unchecked.
    Unobserved,
}

impl Verdict {
    /// The verdict as the catalogue spells it.
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Error => "error",
            Self::Fail => "fail",
            Self::Skip => "skip",
            Self::Unobserved => "unobserved",
        }
    }
}

// A member that is present, whatever it holds: `null` is a message, not an
// absent one.
fn present<'de, D: Deserializer<'de>>(deserializer: D) -> Result<Option<Json>, D::Error> {
    Json::deserialize(deserializer).map(Some)
}

#[derive(serde::Deserialize)]
#[serde(deny_unknown_fields)]
struct ResolutionFile {
    #[allow(dead_code)]
    format: String,
    kind: Option<String>,
    level: Level,
    section: String,
    cases: Vec<RawCase>,
}

#[derive(serde::Deserialize)]
#[serde(deny_unknown_fields)]
struct TreeFile {
    #[allow(dead_code)]
    format: String,
    #[allow(dead_code)]
    kind: String,
    section: String,
    cases: Vec<TreeCase>,
}

#[derive(serde::Deserialize)]
#[serde(deny_unknown_fields, rename_all = "camelCase")]
struct RawCase {
    id: String,
    description: String,
    section: Option<String>,
    #[serde(default, deserialize_with = "present")]
    message: Option<Json>,
    #[serde(default, deserialize_with = "present")]
    payload: Option<Json>,
    #[serde(default, deserialize_with = "present")]
    props: Option<Json>,
    locale: Option<String>,
    #[serde(default, deserialize_with = "present")]
    message_id: Option<Json>,
    modifiers: Option<Map<String, Json>>,
    #[serde(default, deserialize_with = "present")]
    defaults: Option<Json>,
    expected: Option<Expected>,
    generate: Option<String>,
}

#[derive(serde::Deserialize)]
#[serde(deny_unknown_fields)]
struct Manifest {
    format: String,
    version: String,
    files: Vec<ManifestEntry>,
}

#[derive(serde::Deserialize)]
#[serde(deny_unknown_fields)]
struct ManifestEntry {
    path: String,
    level: Option<Level>,
    kind: Option<String>,
    section: String,
    cases: usize,
}

#[derive(serde::Deserialize)]
#[serde(deny_unknown_fields)]
struct Catalogue {
    format: String,
    defects: Vec<DefectEntry>,
}

fn malformed(name: &str, error: impl std::fmt::Display) -> RunError {
    RunError::new(format!(
        "The file {name} does not read as the set's schema: {error}"
    ))
}

impl RawCase {
    fn into_case(self, file: &str) -> Result<Case, RunError> {
        let Self {
            id,
            description,
            section,
            message,
            payload,
            props,
            locale,
            message_id,
            modifiers,
            defaults,
            expected,
            generate,
        } = self;

        if let Some(generate) = generate {
            let written = message.is_some()
                || payload.is_some()
                || props.is_some()
                || locale.is_some()
                || message_id.is_some()
                || modifiers.is_some()
                || defaults.is_some()
                || expected.is_some();

            if written {
                return Err(malformed(
                    file,
                    format!("the generated case {id} writes inputs"),
                ));
            }

            return Ok(Case::Generated(GeneratedCase {
                id,
                description,
                section,
                generate,
            }));
        }

        let (Some(message), Some(expected)) = (message, expected) else {
            return Err(malformed(
                file,
                format!("the case {id} has no message or no expectation"),
            ));
        };
        let modifiers = modifiers
            .map(|named| {
                named
                    .into_iter()
                    .map(|(name, behaviour)| {
                        Behaviour::deserialize(behaviour)
                            .map(|behaviour| (name, behaviour))
                            .map_err(|error| malformed(file, error))
                    })
                    .collect::<Result<Vec<_>, _>>()
            })
            .transpose()?;

        Ok(Case::Concrete(Box::new(ConcreteCase {
            id,
            description,
            section,
            message,
            payload,
            props,
            locale,
            message_id,
            modifiers,
            defaults,
            expected,
        })))
    }
}

/// Reads one fixture file. A file targeting another format is kept as
/// [`FixtureFile::Unknown`], for the plan to refuse.
pub fn read_fixture(name: &str, text: &str) -> Result<Fixture, RunError> {
    let json: Json = serde_json::from_str(text).map_err(|error| malformed(name, error))?;
    let format = json
        .get("format")
        .and_then(Json::as_str)
        .unwrap_or_default();

    if format != FORMAT {
        return Ok(Fixture {
            name: name.to_owned(),
            file: FixtureFile::Unknown {
                format: format.to_owned(),
            },
        });
    }

    let file = if json.get("kind").and_then(Json::as_str) == Some("tree") {
        let tree = TreeFile::deserialize(json).map_err(|error| malformed(name, error))?;

        FixtureFile::Tree {
            section: tree.section,
            cases: tree.cases,
        }
    } else {
        let file = ResolutionFile::deserialize(json).map_err(|error| malformed(name, error))?;

        if file
            .kind
            .as_deref()
            .is_some_and(|kind| kind != "resolution")
        {
            return Err(malformed(name, "its kind is neither resolution nor tree"));
        }

        FixtureFile::Resolution {
            level: file.level,
            section: file.section,
            cases: file
                .cases
                .into_iter()
                .map(|case| case.into_case(name))
                .collect::<Result<_, _>>()?,
        }
    };

    Ok(Fixture {
        name: name.to_owned(),
        file,
    })
}

fn read_catalogue(text: &str) -> Result<Vec<DefectEntry>, RunError> {
    let catalogue: Catalogue =
        serde_json::from_str(text).map_err(|error| malformed("defects.json", error))?;

    if catalogue.format != FORMAT {
        return Err(RunError::new(format!(
            "The catalogue targets the format {}; this runner reads {FORMAT}.",
            catalogue.format
        )));
    }

    Ok(catalogue.defects)
}

fn sha256(bytes: &[u8]) -> String {
    Sha256::digest(bytes)
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
}

fn unpack(archive: &[u8]) -> Result<BTreeMap<String, String>, RunError> {
    let unreadable = |error: zip::result::ZipError| {
        RunError::new(format!("The set's archive does not read: {error}"))
    };
    let mut zip = zip::ZipArchive::new(Cursor::new(archive)).map_err(unreadable)?;
    let mut files = BTreeMap::new();

    for at in 0..zip.len() {
        let mut entry = zip.by_index(at).map_err(unreadable)?;

        if entry.is_dir() {
            continue;
        }

        let name = entry.name().map_err(unreadable)?.into_owned();
        let mut text = String::new();

        entry.read_to_string(&mut text).map_err(|error| {
            RunError::new(format!("The set's file {name} does not read: {error}"))
        })?;
        files.insert(name, text);
    }

    Ok(files)
}

// The manifest is the set's own statement of what it holds: a file it does
// not list, or one whose count, level or section it states otherwise, is a
// set that is not the one released.
fn agrees(manifest: &Manifest, fixtures: &[Fixture]) -> Result<(), RunError> {
    let disagree =
        |what: String| RunError::new(format!("The manifest disagrees with the set: {what}."));

    if manifest.format != FORMAT || manifest.version != VERSION {
        return Err(disagree(format!(
            "it names {} {}",
            manifest.format, manifest.version
        )));
    }

    if manifest.files.len() != fixtures.len() {
        return Err(disagree(format!(
            "it lists {} files, the set holds {}",
            manifest.files.len(),
            fixtures.len()
        )));
    }

    for entry in &manifest.files {
        let name = entry.path.strip_prefix("fixtures/").unwrap_or(&entry.path);
        let fixture = fixtures
            .iter()
            .find(|fixture| fixture.name == name)
            .ok_or_else(|| disagree(format!("{} is not in the set", entry.path)))?;
        let matches = match &fixture.file {
            FixtureFile::Resolution {
                level,
                section,
                cases,
            } => {
                entry.level == Some(*level)
                    && entry.kind.is_none()
                    && *section == entry.section
                    && cases.len() == entry.cases
            }
            FixtureFile::Tree { section, cases } => {
                entry.level.is_none()
                    && entry.kind.as_deref() == Some("tree")
                    && *section == entry.section
                    && cases.len() == entry.cases
            }
            FixtureFile::Unknown { .. } => false,
        };

        if !matches {
            return Err(disagree(format!("{} is listed otherwise", entry.path)));
        }
    }

    Ok(())
}

fn vendored_set() -> Result<Set, RunError> {
    let digest = sha256(ARCHIVE);

    if digest != DIGEST {
        return Err(RunError::new(format!(
            "The vendored archive is not the released set {VERSION}: its sha256 is {digest}, the release states {DIGEST}."
        )));
    }

    let files = unpack(ARCHIVE)?;
    let root = format!("conformance-{VERSION}/");
    let read = |path: &str| {
        files
            .get(&format!("{root}{path}"))
            .ok_or_else(|| RunError::new(format!("The set's archive holds no {path}.")))
    };
    let manifest: Manifest = serde_json::from_str(read("index.json")?)
        .map_err(|error| malformed("index.json", error))?;
    let fixtures = files
        .iter()
        .filter_map(|(path, text)| {
            let name = path.strip_prefix(&format!("{root}fixtures/"))?;

            name.ends_with(".json").then(|| read_fixture(name, text))
        })
        .collect::<Result<Vec<_>, _>>()?;

    agrees(&manifest, &fixtures)?;

    Ok(Set {
        fixtures,
        defects: read_catalogue(read("defects.json")?)?,
    })
}

/// The set this runner carries, release 4.1.0, read once: its archive is
/// checked against the release's digest before anything in it is read, and
/// its manifest against the files it holds.
pub fn vendored() -> Result<&'static Set, RunError> {
    static SET: OnceLock<Result<Set, RunError>> = OnceLock::new();

    SET.get_or_init(vendored_set).as_ref().map_err(Clone::clone)
}

/// The fixture files of a directory, sorted by name, for a set under
/// development; the catalogue is the vendored set's.
pub fn load(directory: &Path) -> Result<Set, RunError> {
    let unreadable = |error: std::io::Error| {
        RunError::new(format!("{} does not read: {error}", directory.display()))
    };
    let mut names: Vec<_> = fs::read_dir(directory)
        .map_err(unreadable)?
        .filter_map(|entry| entry.ok()?.file_name().into_string().ok())
        .filter(|name| name.ends_with(".json"))
        .collect();

    names.sort();

    let fixtures = names
        .iter()
        .map(|name| {
            read_fixture(
                name,
                &fs::read_to_string(directory.join(name)).map_err(unreadable)?,
            )
        })
        .collect::<Result<_, _>>()?;

    Ok(Set {
        fixtures,
        defects: vendored()?.defects.clone(),
    })
}
