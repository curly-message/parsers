use std::borrow::Cow;
use std::error::Error;
use std::fmt;

use crate::props::Props;
use crate::value::Value;
use crate::walk::{Ph, Walker};

/// A modifier a host defines (section 11.3), registered with
/// [`ParserBuilder::modifier`](crate::ParserBuilder::modifier).
///
/// It answers `Ok(Some(value))` with a value that converts to text by section
/// 4, a structured one included. `Ok(None)`, or `Ok(Some(Value::Undefined))`,
/// is no answer: the placeholder takes its fallback chain and nothing is
/// reported. `Err(_)` is a failure, and so is a panic, which is contained:
/// the placeholder takes its fallback chain and reports `failed-modifier`.
///
/// Every closure of the right shape is a modifier:
///
/// ```
/// use curly_message_parser::{Context, ModifierInput, Object, Parser, Value};
///
/// let parser = Parser::builder()
///     .modifier("upper", |input: &ModifierInput<'_>| {
///         Ok(Some(Value::from(input.value().to_ascii_uppercase())))
///     })
///     .build();
/// let payload = Object::new().with("name", "Ada");
///
/// assert_eq!(parser.resolve("{{name:upper}}", &Context::new().payload(&payload)), "ADA");
/// ```
pub trait Modifier: Send + Sync {
    /// Answers for one placeholder.
    fn call(&self, input: &ModifierInput<'_>) -> Result<Option<Value>, ModifierError>;
}

impl<F> Modifier for F
where
    F: Fn(&ModifierInput<'_>) -> Result<Option<Value>, ModifierError> + Send + Sync,
{
    fn call(&self, input: &ModifierInput<'_>) -> Result<Option<Value>, ModifierError> {
        self(input)
    }
}

/// What a modifier is handed (section 11): the value as text, the options as
/// the placeholder writes them, the default, the props composed under the
/// modifier's own name and the locale the caller supplied.
///
/// An option's value and the default are message text, rendered where they
/// are first asked for and not again; an option nobody asks for is never
/// rendered, however deeply it nests. Every accessor borrows the input alone,
/// so a modifier may hold any number of their answers at once.
///
/// The input lives no longer than the call, but the modifier may lend it for
/// the length of its call, through a scoped thread-local say, to host code
/// the parser calls meanwhile: another modifier, a value's `Display`, a
/// report handler. A modifier is *current* while its call is the innermost
/// call of a modifier on the thread that has not returned, through whatever
/// host code runs beneath it (section 13). While this input's modifier is
/// current, a read of an option's value or of the default through it is the
/// walk's, as long as none of them is being rendered: it holds no place.
/// Any other read that renders its text holds one of
/// [`MAX_REENTRY`](crate::MAX_REENTRY) places while it does, as a resolution
/// in progress does, and where none is left it is refused. A read of an
/// option or of the default made from inside its own rendering is refused
/// at once, whoever makes it. A refused read answers the empty string,
/// records nothing, so a later read renders the text, and fails and reports
/// nothing.
pub struct ModifierInput<'w> {
    walker: &'w Walker<'w>,
    placeholder: &'w Ph<'w>,
}

impl<'w> ModifierInput<'w> {
    pub(crate) fn new(walker: &'w Walker<'w>, placeholder: &'w Ph<'w>) -> Self {
        Self {
            walker,
            placeholder,
        }
    }

    /// The value, as text.
    pub fn value(&self) -> &str {
        self.placeholder.value_text().unwrap_or_default()
    }

    /// How many options the placeholder declares, in source order, a key
    /// written twice counted twice. The inline default is none of them.
    pub fn options(&self) -> usize {
        self.placeholder.options().len()
    }

    /// The key of the option at `at`, escapes removed.
    pub fn option_key(&self, at: usize) -> Option<&str> {
        self.placeholder
            .options()
            .get(at)
            .map(|option| &*option.key)
    }

    /// The value of the option at `at`, rendered on the first ask (section
    /// 9.4). An option that writes no value stands for its key. The empty
    /// string where the read is refused, as the type's own page says.
    pub fn option_value(&self, at: usize) -> Option<&str> {
        self.walker.option_value(self.placeholder, at)
    }

    /// The placeholder's fallback chain (section 10), walked on the first
    /// ask: a link nobody asks for is never read, and never reported. The
    /// empty string where the read is refused, as the type's own page says.
    pub fn default(&self) -> &str {
        self.walker.default_text(self.placeholder)
    }

    /// The props the layers hold under this modifier's name (section 11.2):
    /// the parser's modifier defaults, then the caller's props, then the
    /// wrapper's, each overriding the properties it names. `Undefined` names
    /// nothing. Empty where no layer holds an object under the name.
    pub fn props(&self) -> Props<'_> {
        self.walker.props(self.placeholder)
    }

    /// The locale as the caller supplied it, the empty string included, or
    /// nothing where it supplied none.
    pub fn locale(&self) -> Option<&str> {
        self.walker.locale()
    }
}

impl fmt::Debug for ModifierInput<'_> {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("ModifierInput")
            .field("value", &self.value())
            .field("options", &self.options())
            .field("locale", &self.locale())
            .finish_non_exhaustive()
    }
}

/// A modifier's failure. The placeholder takes its fallback chain and
/// reports `failed-modifier`; what the error says stays the host's, since a
/// report carries message text alone.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ModifierError {
    message: Cow<'static, str>,
}

impl ModifierError {
    /// A failure, described.
    pub fn new(message: impl Into<Cow<'static, str>>) -> Self {
        Self {
            message: message.into(),
        }
    }
}

impl fmt::Display for ModifierError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&self.message)
    }
}

impl Error for ModifierError {}
