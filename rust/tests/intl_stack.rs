#![cfg(feature = "intl")]
#![allow(clippy::unwrap_used, clippy::panic, clippy::arithmetic_side_effects)]

// The worst case of section 13 for the thread's stack with the format's own
// facility. jiff reads a zone's rules the first time the process formats in
// a name of it, whichever thread does so, and each such first use costs a
// debug build about 230 KiB of stack, so this test is a binary of its own:
// nothing before it in the process formats in America/New_York, the zone
// name it formats in.

use std::fmt;
use std::panic::AssertUnwindSafe;
use std::sync::{Arc, Mutex};

use curly_message_parser::{
    Context, IcuIntl, MAX_NESTING, MAX_REENTRY, ModifierError, ModifierInput, Object, Parser,
    ReentryLimit, Report, Value,
};

// A `timeZone` that describes itself by resolving the message that reads it
// again, over the same payload and props, as long as that is not refused, and
// then names a zone.
type Resolution = Arc<Mutex<Option<(Parser, Object, Object, String)>>>;

struct Named {
    resolution: Resolution,
    refused: Arc<Mutex<usize>>,
}

impl fmt::Display for Named {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        let (parser, payload, props, message) = self.resolution.lock().unwrap().clone().unwrap();
        let again = std::panic::catch_unwind(AssertUnwindSafe(|| {
            parser.resolve(
                message.as_str(),
                &Context::new().payload(&payload).props(&props).locale("en"),
            )
        }));

        if let Err(refusal) = again {
            assert_eq!(refusal.downcast_ref(), Some(&ReentryLimit));
            *self.refused.lock().unwrap() += 1;
        }

        formatter.write_str("America/New_York")
    }
}

// `MAX_REENTRY` resolutions in progress, each waiting on its date request's
// `timeZone` as deep as a placeholder resolves, and the innermost formatting
// in a zone name the process has not used yet. It runs on the thread the
// README says that needs.
#[test]
fn a_date_reaching_itself_at_full_nesting_fits_the_documented_stack() {
    std::thread::Builder::new()
        .stack_size(1024 * 1024)
        .spawn(|| {
            let calls: Arc<Mutex<usize>> = Arc::default();
            let counted = Arc::clone(&calls);
            let reports: Arc<Mutex<Vec<&'static str>>> = Arc::default();
            let sink = Arc::clone(&reports);
            let parser = Parser::builder()
                .intl(IcuIntl::new())
                .on_report(move |report: &Report| sink.lock().unwrap().push(report.code.as_str()))
                .modifier("deeper", move |input: &ModifierInput<'_>| {
                    *counted.lock().unwrap() += 1;

                    Ok::<_, ModifierError>(Some(Value::from(
                        input.option_value(0).unwrap_or_default(),
                    )))
                })
                .build();
            let message = (1..MAX_NESTING).fold("{{n:date}}".to_owned(), |inner, _| {
                format!("{{{{v:deeper; a:{inner}}}}}")
            });
            let resolution: Resolution = Arc::default();
            let refused: Arc<Mutex<usize>> = Arc::default();
            let zone = Value::host(Named {
                resolution: Arc::clone(&resolution),
                refused: Arc::clone(&refused),
            });
            let payload = Object::new().with("v", "a").with("n", 0);
            let props = Object::new().with(
                "date",
                Object::new()
                    .with("dateStyle", "full")
                    .with("timeStyle", "full")
                    .with("timeZone", zone),
            );

            *resolution.lock().unwrap() = Some((
                parser.clone(),
                payload.clone(),
                props.clone(),
                message.clone(),
            ));

            assert_eq!(
                parser.resolve(
                    message.as_str(),
                    &Context::new().payload(&payload).props(&props).locale("en")
                ),
                "Wednesday, December 31, 1969 at 7:00:00\u{202f}PM Eastern Standard Time"
            );
            assert_eq!(*refused.lock().unwrap(), 1);
            assert_eq!(*calls.lock().unwrap(), MAX_REENTRY * (MAX_NESTING - 1));
            assert!(reports.lock().unwrap().is_empty());

            *resolution.lock().unwrap() = None;
        })
        .unwrap()
        .join()
        .unwrap();
}
