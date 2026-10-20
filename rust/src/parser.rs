use std::fmt;
use std::sync::Arc;

use crate::convert::{Converted, Record, Txt};
use crate::depth::contained;
use crate::report::Report;
use crate::value::{Object, Value};
use crate::walk::Walker;

type OnReport = Box<dyn Fn(&Report) + Send + Sync>;

pub(crate) struct Config {
    pub(crate) on_report: Option<OnReport>,
    pub(crate) recognize_wrappers: bool,
}

/// Resolves messages (section 5). It is cheap to clone and may be shared
/// across threads: what it holds is its configuration, and every resolution
/// keeps its own state.
#[derive(Clone)]
pub struct Parser {
    config: Arc<Config>,
}

/// Configures a [`Parser`].
#[must_use]
pub struct ParserBuilder {
    on_report: Option<OnReport>,
    recognize_wrappers: bool,
}

/// What a resolution reads besides its message. Every input is optional.
#[derive(Clone, Copy, Debug, Default)]
pub struct Context<'a> {
    payload: Option<&'a Object>,
    id: Option<&'a str>,
}

/// A message to resolve: text, or a value that converts to text by section 4
/// before anything reads it.
#[derive(Clone, Copy, Debug)]
pub enum Message<'a> {
    /// The message as written.
    Text(&'a str),
    /// A value standing for its text.
    Value(&'a Value),
    /// No message at all.
    Absent,
}

/// A conformance level of section 2.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub enum Level {
    /// The syntax, the resolution and the comparisons.
    Core,
    /// The formatting modifiers and the plural selections.
    Intl,
    /// Modifiers a host defines.
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

impl Parser {
    /// A parser with every option at its default.
    pub fn new() -> Self {
        Self::builder().build()
    }

    /// A builder to configure a parser with.
    pub fn builder() -> ParserBuilder {
        ParserBuilder {
            on_report: None,
            recognize_wrappers: true,
        }
    }

    /// The levels this parser claims (section 2).
    pub fn levels(&self) -> &'static [Level] {
        &[Level::Core]
    }

    /// Resolves a message against a context. Resolution never fails: what it
    /// cannot resolve falls back, and is reported where a handler was given.
    pub fn resolve<'m>(&self, message: impl Into<Message<'m>>, context: &Context<'_>) -> String {
        let message = message.into();

        // Whatever escapes every containment inside is a defect of this
        // crate, and still no reason to fail the caller's render.
        contained(|| self.walk(message, context)).unwrap_or_default()
    }

    fn walk(&self, message: Message<'_>, context: &Context<'_>) -> String {
        let record = Record::default();
        // A message nothing describes is a message nobody wrote, and nothing
        // behind it is read on its account.
        let text = match message {
            Message::Text(text) => Txt::Borrowed(text),
            Message::Value(value) => match record.convert(value) {
                Converted::Text(text) => text,
                Converted::Absent | Converted::Undescribable => return String::new(),
            },
            Message::Absent => return String::new(),
        };
        let walker = Walker::new(&self.config, &text, context.payload, context.id, record);

        walker.render(0, text.len(), 1).unwrap_or_default()
    }
}

impl Default for Parser {
    fn default() -> Self {
        Self::new()
    }
}

impl fmt::Debug for Parser {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("Parser")
            .field("levels", &self.levels())
            .field("recognize_wrappers", &self.config.recognize_wrappers)
            .field("on_report", &self.config.on_report.is_some())
            .finish()
    }
}

impl ParserBuilder {
    /// Where reports go (section 14.2). A handler that panics is contained,
    /// and the resolution carries on.
    pub fn on_report(mut self, handler: impl Fn(&Report) + Send + Sync + 'static) -> Self {
        self.on_report = Some(Box::new(handler));
        self
    }

    /// Whether a payload entry shaped like a wrapper configures its value
    /// (section 4.1). It does unless the caller says otherwise.
    pub fn recognize_wrappers(mut self, recognize: bool) -> Self {
        self.recognize_wrappers = recognize;
        self
    }

    /// The parser.
    pub fn build(self) -> Parser {
        Parser {
            config: Arc::new(Config {
                on_report: self.on_report,
                recognize_wrappers: self.recognize_wrappers,
            }),
        }
    }
}

impl fmt::Debug for ParserBuilder {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("ParserBuilder")
            .field("recognize_wrappers", &self.recognize_wrappers)
            .field("on_report", &self.on_report.is_some())
            .finish()
    }
}

impl<'a> Context<'a> {
    /// A context holding nothing.
    pub fn new() -> Self {
        Self::default()
    }

    /// The payload placeholders read their values from.
    #[must_use]
    pub fn payload(mut self, payload: &'a Object) -> Self {
        self.payload = Some(payload);
        self
    }

    /// The id the caller knows the message by, carried by every report.
    #[must_use]
    pub fn id(mut self, id: &'a str) -> Self {
        self.id = Some(id);
        self
    }
}

impl<'a> From<&'a str> for Message<'a> {
    fn from(text: &'a str) -> Self {
        Self::Text(text)
    }
}

impl<'a> From<&'a String> for Message<'a> {
    fn from(text: &'a String) -> Self {
        Self::Text(text)
    }
}

impl<'a> From<&'a Value> for Message<'a> {
    fn from(value: &'a Value) -> Self {
        Self::Value(value)
    }
}

impl<'a, T: Into<Message<'a>>> From<Option<T>> for Message<'a> {
    fn from(message: Option<T>) -> Self {
        message.map_or(Self::Absent, Into::into)
    }
}
