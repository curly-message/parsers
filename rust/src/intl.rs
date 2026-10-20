use std::borrow::Cow;
use std::error::Error;
use std::fmt;

use crate::props::Props;

/// The internationalization facility the formatting modifiers and the plural
/// selections delegate to (sections 11.2 and 11.5), configured with
/// [`ParserBuilder::intl`](crate::ParserBuilder::intl).
///
/// A request is stated in the vocabulary of ECMA-402, which is the one the
/// format's properties are written in: the facility, the options it is
/// constructed with, and the input. Neither call is made without a locale,
/// and each is contained: a refusal, or a panic, is a modifier that could not
/// answer, so the placeholder takes its fallback chain and reports
/// `failed-modifier`.
///
/// A supplier of one's own may stand in for [`IcuIntl`](crate::IcuIntl),
/// whose locale data is large, or record the requests a resolution makes:
///
/// ```
/// use curly_message_parser::{Category, Context, Intl, Object, Parser, Refusal, Request};
///
/// struct English;
///
/// impl Intl for English {
///     fn format(&self, _locale: &str, _request: &Request<'_>) -> Result<String, Refusal> {
///         Err(Refusal::new("formats nothing"))
///     }
///
///     fn select(&self, _locale: &str, request: &Request<'_>) -> Result<Category, Refusal> {
///         match request.number() {
///             Some(n) if n == 1.0 => Ok(Category::One),
///             _ => Ok(Category::Other),
///         }
///     }
/// }
///
/// let parser = Parser::builder().intl(English).build();
/// let payload = Object::new().with("n", 3);
/// let context = Context::new().payload(&payload).locale("en");
///
/// assert_eq!(parser.resolve("{{n:plural; one:file; other:files}}", &context), "files");
/// ```
pub trait Intl: Send + Sync {
    /// The text a `NumberFormat`, `DateTimeFormat` or `RelativeTimeFormat`
    /// request formats to.
    fn format(&self, locale: &str, request: &Request<'_>) -> Result<String, Refusal>;

    /// The plural category a `PluralRules` request selects.
    fn select(&self, locale: &str, request: &Request<'_>) -> Result<Category, Refusal>;
}

/// One request to the facility: which one, the options it is constructed
/// with, and the input it is handed. The options are read where the layers
/// hold them, so a request lives no longer than the resolution that makes it;
/// [`Props::to_object`] keeps them.
#[derive(Clone, Debug)]
#[non_exhaustive]
pub struct Request<'a> {
    /// The facility.
    pub api: Api,
    /// The properties it reads, as the layers of section 11.2 compose them,
    /// with what the modifier sets itself (see [`Props`]).
    pub options: Props<'a>,
    /// What it formats or selects for.
    pub input: Input,
}

impl<'a> Request<'a> {
    /// A request, its options read from what `options` holds:
    /// `Request::new(api, &object, input)`.
    pub fn new(api: Api, options: impl Into<Props<'a>>, input: Input) -> Self {
        Self {
            api,
            options: options.into(),
            input,
        }
    }

    /// The input, where it is a number.
    pub fn number(&self) -> Option<f64> {
        match self.input {
            Input::Number(number) => Some(number),
            Input::Relative(..) => None,
        }
    }
}

/// The facilities of ECMA-402 the format's modifiers delegate to.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub enum Api {
    /// `Intl.NumberFormat`: `number` and `currency`.
    NumberFormat,
    /// `Intl.DateTimeFormat`: `date`, over milliseconds since the epoch.
    DateTimeFormat,
    /// `Intl.RelativeTimeFormat`: `ago`.
    RelativeTimeFormat,
    /// `Intl.PluralRules`: `plural` and `ordinal`.
    PluralRules,
}

impl Api {
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

/// What a request formats or selects for.
#[derive(Clone, Copy, Debug, PartialEq)]
pub enum Input {
    /// A number, or for `DateTimeFormat` milliseconds since the epoch.
    Number(f64),
    /// A count of a unit, signed: negative is in the past.
    Relative(f64, Unit),
}

/// The units `ago` counts in, the rungs of its ladder.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub enum Unit {
    /// Seconds.
    Second,
    /// Minutes.
    Minute,
    /// Hours.
    Hour,
    /// Days.
    Day,
    /// Weeks.
    Week,
    /// Months.
    Month,
    /// Years.
    Year,
}

impl Unit {
    /// The unit as ECMA-402 spells it.
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Second => "second",
            Self::Minute => "minute",
            Self::Hour => "hour",
            Self::Day => "day",
            Self::Week => "week",
            Self::Month => "month",
            Self::Year => "year",
        }
    }
}

/// A plural category of Unicode's plural rules.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub enum Category {
    /// `zero`.
    Zero,
    /// `one`.
    One,
    /// `two`.
    Two,
    /// `few`.
    Few,
    /// `many`.
    Many,
    /// `other`.
    Other,
}

impl Category {
    /// The category as a selection's option key writes it.
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Zero => "zero",
            Self::One => "one",
            Self::Two => "two",
            Self::Few => "few",
            Self::Many => "many",
            Self::Other => "other",
        }
    }
}

/// A request the facility refuses: what ECMA-402 raises a `RangeError` or a
/// `TypeError` for, or a locale it cannot read.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Refusal {
    message: Cow<'static, str>,
}

impl Refusal {
    /// A refusal, described.
    pub fn new(message: impl Into<Cow<'static, str>>) -> Self {
        Self {
            message: message.into(),
        }
    }
}

impl fmt::Display for Refusal {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&self.message)
    }
}

impl Error for Refusal {}

/// A property a facility reads and cannot express (section 11.2): a request
/// naming it formats with what the facility does express. Where `when` names
/// properties, the entry holds only for a request holding each of them with
/// that value.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Unexpressible {
    /// The facility that reads the property.
    pub api: Api,
    /// The property.
    pub property: &'static str,
    /// The values other properties hold where the entry holds.
    pub when: &'static [(&'static str, &'static str)],
}
