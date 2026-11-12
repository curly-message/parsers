//! Every formatting request the vendored set states, once each, as the JSON
//! list `golden.mjs` performs on Node:
//!
//! ```text
//! cargo run -p curly-message-conformance --bin requests | node conformance/golden.mjs > conformance/set/golden-4.1.0.json
//! ```

use std::collections::HashSet;
use std::io::Write;
use std::process::ExitCode;

use curly_message_conformance::{Case, FixtureFile, vendored};
use serde_json::{Map, Value as Json};

fn requests() -> Result<Vec<Json>, String> {
    let set = vendored().map_err(|error| error.to_string())?;
    let mut seen = HashSet::new();
    let mut listed = Vec::new();

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
            let mut request = Map::new();

            request.insert("name".to_owned(), case.id.clone().into());

            if let Some(locale) = &case.locale {
                request.insert("locale".to_owned(), locale.clone().into());
            }

            request.insert("api".to_owned(), format.api.as_str().into());

            if let Some(options) = &format.options {
                request.insert("options".to_owned(), options.clone());
            }

            request.insert("input".to_owned(), format.input.clone());

            let looked_up = (
                case.locale.clone(),
                format.api.as_str(),
                format.options.as_ref().map(Json::to_string),
                format.input.to_string(),
            );

            if seen.insert(looked_up) {
                listed.push(Json::Object(request));
            }
        }
    }

    Ok(listed)
}

fn main() -> ExitCode {
    let written = requests().and_then(|listed| {
        let mut out = std::io::stdout().lock();

        serde_json::to_writer_pretty(&mut out, &listed)
            .map_err(|error| error.to_string())
            .and_then(|()| writeln!(out).map_err(|error| error.to_string()))
    });

    match written {
        Ok(()) => ExitCode::SUCCESS,
        Err(error) => {
            let _ = writeln!(std::io::stderr(), "{error}");

            ExitCode::FAILURE
        }
    }
}
