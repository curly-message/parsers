use std::cell::Cell;
use std::panic::{AssertUnwindSafe, catch_unwind};

use crate::MAX_HOST_DEPTH;

// Stack exhaustion is no panic: it aborts the process. So what a thread's
// stack holds of the walk is counted, across every resolution and every
// parser running on it, and a walk that would go past the bound does not
// start (section 13).
thread_local! {
    static ACTIVE: Cell<usize> = const { Cell::new(0) };
    static EXHAUSTED: Cell<bool> = const { Cell::new(false) };
}

/// One walk activation on this thread's stack, counted while it lives. The
/// count is lowered on drop, so a panic unwinding through a contained host
/// call leaves it right.
pub(crate) struct Activation {
    counted: bool,
}

impl Activation {
    /// An activation, or nothing where this thread already holds
    /// [`MAX_HOST_DEPTH`] of them. A refusal marks the thread, so the host call
    /// it happened inside is answered as one that did not come back.
    pub(crate) fn enter() -> Option<Self> {
        let entered = ACTIVE.try_with(|active| {
            let count = active.get();

            if count >= MAX_HOST_DEPTH {
                return false;
            }

            active.set(count.saturating_add(1));

            true
        });

        match entered {
            Ok(true) => Some(Self { counted: true }),
            Ok(false) => {
                let _ = EXHAUSTED.try_with(|exhausted| exhausted.set(true));

                None
            }
            // A thread tearing its locals down keeps no count. A resolution
            // there is still bounded by its own nesting limit.
            Err(_) => Some(Self { counted: false }),
        }
    }
}

impl Drop for Activation {
    fn drop(&mut self) {
        if self.counted {
            let _ = ACTIVE.try_with(|active| active.set(active.get().saturating_sub(1)));
        }
    }
}

/// Runs `call`, answering nothing where it panicked. A panic's payload is
/// dropped under the same containment, since dropping it runs host code too.
pub(crate) fn contained<T>(call: impl FnOnce() -> T) -> Option<T> {
    match catch_unwind(AssertUnwindSafe(call)) {
        Ok(answer) => Some(answer),
        Err(payload) => {
            let _ = catch_unwind(AssertUnwindSafe(move || drop(payload)));

            None
        }
    }
}

/// Calls host code: a report handler, a value's `Display`, a modifier. The
/// answer is nothing where the call panicked, or where a walk it started was
/// refused for want of stack: then it did not come back, whatever it
/// returned. A mark left by a call further out is kept for that call.
///
/// No borrow of a walker's state is held across it.
pub(crate) fn host_call<T>(call: impl FnOnce() -> T) -> Option<T> {
    let outer = EXHAUSTED
        .try_with(|exhausted| exhausted.replace(false))
        .unwrap_or(false);
    let answer = contained(call);
    let exhausted = EXHAUSTED
        .try_with(|exhausted| exhausted.replace(outer))
        .unwrap_or(false);

    if exhausted {
        if let Some(answer) = answer {
            let _ = contained(move || drop(answer));
        }

        return None;
    }

    answer
}
