#![cfg(feature = "intl")]
#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::arithmetic_side_effects
)]

// `IcuIntl` against Node on one request per option mapping of `IcuIntl`
// (`tests/intl/requests.json`), with Node's answers recorded by
// `tests/intl/golden.mjs` in `tests/intl/golden.json`. A request whose answer
// differs from Node's is listed below with the reason, so a new difference is
// seen rather than shipped.

use std::collections::BTreeMap;

use curly_message_parser::{
    Api, Category, IcuIntl, Input, Intl, Object, Request, UNEXPRESSIBLE, Unit, Value,
};

const REQUESTS: &str = include_str!("intl/requests.json");
const GOLDEN: &str = include_str!("intl/golden.json");

const CURRENCY_DIGITS_REASON: &str =
    "ICU4X formats a currency amount with the currency's own digits (UNEXPRESSIBLE)";
const UNPADDED_REASON: &str =
    "ICU4X pads day and month: no ICU4X 2.3 field set selects CLDR's yMd skeleton";
const ERA_REASON: &str = "ICU4X shows the era before year 1, which Node omits";
const FULL_YEAR_REASON: &str =
    "ICU4X writes a short date's year in full outside 1950 to 2049; Node writes two digits";

const DIFFERENCES: &[(&str, &str)] = &[
    ("currency maximumFractionDigits", CURRENCY_DIGITS_REASON),
    ("currency maximumSignificantDigits", CURRENCY_DIGITS_REASON),
    ("currency minimumFractionDigits", CURRENCY_DIGITS_REASON),
    ("currency minimumIntegerDigits", CURRENCY_DIGITS_REASON),
    ("currency minimumSignificantDigits", CURRENCY_DIGITS_REASON),
    ("currency roundingIncrement", CURRENCY_DIGITS_REASON),
    ("currency roundingMode", CURRENCY_DIGITS_REASON),
    ("currency roundingPriority", CURRENCY_DIGITS_REASON),
    ("currency trailingZeroDisplay", CURRENCY_DIGITS_REASON),
    (
        "currency useGrouping",
        "ICU4X groups a currency amount by the locale's default (UNEXPRESSIBLE)",
    ),
    ("date before jiff Europe/Prague", ERA_REASON),
    ("date before year 1", ERA_REASON),
    (
        "date full time cs",
        "CLDR pattern choice: ICU4X writes no comma before a Czech zone name",
    ),
    ("date no style cs", UNPADDED_REASON),
    ("date no style de", UNPADDED_REASON),
    ("date no style ja", UNPADDED_REASON),
    ("date no style nl", UNPADDED_REASON),
    ("date no style pl", UNPADDED_REASON),
    ("date short year 1949", FULL_YEAR_REASON),
    ("date short year 2050", FULL_YEAR_REASON),
    ("date short year 2050 de", FULL_YEAR_REASON),
    (
        "number numberingSystem arab",
        "ICU4X's English data for the arab numbering system keeps the Latin grouping separator",
    ),
];

// V8 writes a space where CLDR has a narrow no-break space in a time, before
// its day period; ICU4X writes CLDR's.
const NARROW_SPACE: char = '\u{202f}';

#[derive(Clone, Debug, PartialEq)]
enum Json {
    Null,
    Bool(bool),
    Number(f64),
    String(String),
    Array(Vec<Json>),
    Object(Vec<(String, Json)>),
}

impl Json {
    fn get(&self, key: &str) -> Option<&Json> {
        match self {
            Json::Object(members) => members
                .iter()
                .find(|(name, _)| name == key)
                .map(|(_, value)| value),
            _ => None,
        }
    }

    fn text(&self, key: &str) -> &str {
        match self.get(key) {
            Some(Json::String(text)) => text,
            other => panic!("`{key}` is no string: {other:?}"),
        }
    }

    fn items(&self) -> &[Json] {
        match self {
            Json::Array(items) => items,
            other => panic!("no array: {other:?}"),
        }
    }

    fn without(&self, key: &str) -> Json {
        match self {
            Json::Object(members) => Json::Object(
                members
                    .iter()
                    .filter(|(name, _)| name != key)
                    .cloned()
                    .collect(),
            ),
            other => other.clone(),
        }
    }

    fn value(&self) -> Value {
        match self {
            Json::Null => Value::Null,
            Json::Bool(boolean) => Value::Bool(*boolean),
            Json::Number(number) => Value::Number(*number),
            Json::String(text) => Value::from(text.as_str()),
            Json::Array(items) => Value::from(items.iter().map(Json::value).collect::<Vec<_>>()),
            Json::Object(_) => Value::from(self.object()),
        }
    }

    fn object(&self) -> Object {
        match self {
            Json::Object(members) => members.iter().fold(Object::new(), |object, (name, value)| {
                object.with(name.as_str(), value.value())
            }),
            other => panic!("no object: {other:?}"),
        }
    }
}

// The JSON the two files are written in, read in full: both are this
// repository's own, so a malformed one is a failure of the test.
struct Reader<'a> {
    text: &'a [u8],
    at: usize,
}

impl Reader<'_> {
    fn read(text: &str) -> Json {
        let mut reader = Reader {
            text: text.as_bytes(),
            at: 0,
        };
        let value = reader.value();

        reader.space();
        assert_eq!(reader.at, reader.text.len(), "text after the value");

        value
    }

    fn space(&mut self) {
        while self.text.get(self.at).is_some_and(u8::is_ascii_whitespace) {
            self.at += 1;
        }
    }

    fn eat(&mut self, byte: u8) -> bool {
        self.space();

        let found = self.text.get(self.at) == Some(&byte);

        if found {
            self.at += 1;
        }

        found
    }

    fn word(&mut self, word: &str, value: Json) -> Json {
        assert!(
            self.text[self.at..].starts_with(word.as_bytes()),
            "no `{word}` at {}",
            self.at
        );
        self.at += word.len();

        value
    }

    fn value(&mut self) -> Json {
        self.space();

        match self.text[self.at] {
            b'n' => self.word("null", Json::Null),
            b't' => self.word("true", Json::Bool(true)),
            b'f' => self.word("false", Json::Bool(false)),
            b'"' => Json::String(self.string()),
            b'[' => {
                self.at += 1;

                let mut items = Vec::new();

                if !self.eat(b']') {
                    loop {
                        items.push(self.value());

                        if self.eat(b']') {
                            break;
                        }

                        assert!(self.eat(b','), "no `,` at {}", self.at);
                    }
                }

                Json::Array(items)
            }
            b'{' => {
                self.at += 1;

                let mut members = Vec::new();

                if !self.eat(b'}') {
                    loop {
                        self.space();

                        let name = self.string();

                        assert!(self.eat(b':'), "no `:` at {}", self.at);
                        members.push((name, self.value()));

                        if self.eat(b'}') {
                            break;
                        }

                        assert!(self.eat(b','), "no `,` at {}", self.at);
                    }
                }

                Json::Object(members)
            }
            _ => {
                let start = self.at;

                while self.text.get(self.at).is_some_and(|byte| {
                    matches!(byte, b'-' | b'+' | b'.' | b'e' | b'E' | b'0'..=b'9')
                }) {
                    self.at += 1;
                }

                let number = std::str::from_utf8(&self.text[start..self.at]).unwrap();

                Json::Number(
                    number
                        .parse()
                        .unwrap_or_else(|_| panic!("no number at {start}")),
                )
            }
        }
    }

    fn string(&mut self) -> String {
        assert_eq!(self.text[self.at], b'"', "no string at {}", self.at);
        self.at += 1;

        let mut units = Vec::new();
        let mut text = String::new();

        loop {
            let byte = self.text[self.at];

            self.at += 1;

            match byte {
                b'"' => break,
                b'\\' => {
                    let escaped = self.text[self.at];

                    self.at += 1;

                    match escaped {
                        b'u' => {
                            let hex =
                                std::str::from_utf8(&self.text[self.at..self.at + 4]).unwrap();

                            units.push(u16::from_str_radix(hex, 16).unwrap());
                            self.at += 4;

                            continue;
                        }
                        b'n' => units.push(u16::from(b'\n')),
                        b't' => units.push(u16::from(b'\t')),
                        b'r' => units.push(u16::from(b'\r')),
                        b'b' => units.push(0x08),
                        b'f' => units.push(0x0c),
                        other => units.push(u16::from(other)),
                    }
                }
                other => {
                    assert!(other.is_ascii(), "a byte outside ASCII at {}", self.at);
                    units.push(u16::from(other));
                }
            }
        }

        text.push_str(&String::from_utf16(&units).unwrap());

        text
    }
}

fn api(name: &str) -> Api {
    match name {
        "NumberFormat" => Api::NumberFormat,
        "DateTimeFormat" => Api::DateTimeFormat,
        "RelativeTimeFormat" => Api::RelativeTimeFormat,
        "PluralRules" => Api::PluralRules,
        other => panic!("no such facility: {other}"),
    }
}

fn unit(name: &str) -> Unit {
    [
        Unit::Second,
        Unit::Minute,
        Unit::Hour,
        Unit::Day,
        Unit::Week,
        Unit::Month,
        Unit::Year,
    ]
    .into_iter()
    .find(|unit| unit.as_str() == name)
    .unwrap_or_else(|| panic!("no such unit: {name}"))
}

// A request's facility, options and input.
fn request(entry: &Json) -> (Api, Object, Input) {
    let input = match entry.get("input") {
        Some(Json::Number(number)) => Input::Number(*number),
        Some(Json::Array(pair)) => match pair.as_slice() {
            [Json::Number(count), Json::String(name)] => Input::Relative(*count, unit(name)),
            other => panic!("no relative input: {other:?}"),
        },
        other => panic!("no input: {other:?}"),
    };

    (
        api(entry.text("api")),
        entry.get("options").unwrap().object(),
        input,
    )
}

// What `IcuIntl` answers, in the shape the golden file records Node's.
fn answer(entry: &Json) -> Json {
    let (api, options, input) = request(entry);
    let request = Request::new(api, &options, input);
    let locale = entry.text("locale");
    let answered = if request.api == Api::PluralRules {
        IcuIntl::new()
            .select(locale, &request)
            .map(|category: Category| ("category", category.as_str().to_owned()))
    } else {
        IcuIntl::new()
            .format(locale, &request)
            .map(|text| ("text", text))
    };

    match answered {
        Ok((kind, text)) => Json::Object(vec![(kind.to_owned(), Json::String(text))]),
        Err(_) => Json::Object(vec![("error".to_owned(), Json::Bool(true))]),
    }
}

// Node names the error it raised; a refusal is compared as a refusal. A
// date's text is compared as V8 spells it.
fn agrees(api: &str, icu: &Json, node: &Json) -> bool {
    match (icu.get("error"), node.get("error"), icu.get("text")) {
        (Some(_), Some(_), _) => true,
        (None, None, Some(Json::String(text))) if api == "DateTimeFormat" => {
            node.get("text") == Some(&Json::String(text.replace(NARROW_SPACE, " ")))
        }
        (None, None, _) => icu == node,
        _ => false,
    }
}

fn answers() -> Vec<Json> {
    Reader::read(GOLDEN)
        .get("answers")
        .unwrap()
        .items()
        .to_vec()
}

fn differences() -> BTreeMap<String, (Json, Json)> {
    answers()
        .iter()
        .filter_map(|entry| {
            let node = entry.get("answer").unwrap();
            let icu = answer(entry);

            (!agrees(entry.text("api"), &icu, node))
                .then(|| (entry.text("name").to_owned(), (icu, node.clone())))
        })
        .collect()
}

#[test]
fn the_golden_file_answers_the_requests_as_they_stand() {
    let requests = Reader::read(REQUESTS);
    let answered: Vec<Json> = answers()
        .iter()
        .map(|entry| entry.without("answer"))
        .collect();

    assert_eq!(
        requests.items(),
        answered.as_slice(),
        "run tests/intl/golden.mjs again"
    );
}

#[test]
fn every_request_has_a_name_of_its_own() {
    let answers = answers();
    let mut names: Vec<&str> = answers.iter().map(|entry| entry.text("name")).collect();
    let count = names.len();

    names.sort_unstable();
    names.dedup();

    assert_eq!(names.len(), count);
}

#[test]
fn icu_answers_as_node_does_but_where_listed() {
    let found = differences();
    let listed: BTreeMap<&str, &str> = DIFFERENCES.iter().copied().collect();
    let unlisted: Vec<_> = found
        .iter()
        .filter(|(name, _)| !listed.contains_key(name.as_str()))
        .map(|(name, (icu, node))| format!("{name}: {icu:?} where Node has {node:?}"))
        .collect();
    let agreeing: Vec<_> = listed
        .keys()
        .filter(|name| !found.contains_key(**name))
        .collect();

    assert!(
        unlisted.is_empty(),
        "differences not listed:\n{}",
        unlisted.join("\n")
    );
    assert!(agreeing.is_empty(), "listed but agreeing: {agreeing:?}");
}

// The properties `IcuIntl` reads under `style: "currency"` and could fail to
// express there, since its currency formatter applies the currency's own
// digits: each has a request of its own, named `currency <property>`.
const CURRENCY_DIGITS: [&str; 11] = [
    "minimumIntegerDigits",
    "minimumFractionDigits",
    "maximumFractionDigits",
    "minimumSignificantDigits",
    "maximumSignificantDigits",
    "roundingIncrement",
    "roundingMode",
    "roundingPriority",
    "trailingZeroDisplay",
    "useGrouping",
    "signDisplay",
];

// `UNEXPRESSIBLE`'s currency row holds the properties whose request differs
// from Node's, and no other: one whose request agrees is expressed.
#[test]
fn the_currency_row_holds_each_property_icu_does_not_express() {
    let found = differences();
    let answers = answers();
    let declared: Vec<&str> = UNEXPRESSIBLE
        .iter()
        .filter(|entry| entry.when == [("style", "currency")])
        .map(|entry| entry.property)
        .collect();
    let unexpressed: Vec<&str> = CURRENCY_DIGITS
        .into_iter()
        .filter(|property| {
            let name = format!("currency {property}");
            let entry = answers
                .iter()
                .find(|entry| entry.text("name") == name)
                .unwrap_or_else(|| panic!("no request named `{name}`"));

            assert!(entry.get("options").unwrap().get(property).is_some());

            found.contains_key(&name)
        })
        .collect();

    assert_eq!(declared, unexpressed);
}

#[test]
fn a_date_is_compared_as_v8_spells_it_and_only_a_date() {
    let answers = answers();
    let spelled = |api: &str| {
        answers.iter().filter(move |entry| entry.text("api") == api).any(|entry| {
            matches!(answer(entry).get("text"), Some(Json::String(text)) if text.contains(NARROW_SPACE))
        })
    };
    let node_spells = answers.iter().any(|entry| {
        matches!(
            entry.get("answer").unwrap().get("text"),
            Some(Json::String(text)) if text.contains(NARROW_SPACE)
        )
    });

    assert!(spelled("DateTimeFormat"));
    assert!(!spelled("NumberFormat") && !spelled("RelativeTimeFormat"));
    assert!(!node_spells);
}
