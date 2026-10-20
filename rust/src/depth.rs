use std::cell::Cell;
use std::fmt;
use std::marker::PhantomData;
use std::panic::{AssertUnwindSafe, catch_unwind};

use crate::MAX_REENTRY;

// Stack exhaustion is no panic: it aborts the process. So the resolutions in
// progress on a thread are counted, across every parser running on it, and
// one asked for past the bound is refused (section 13). One resolution's own
// walk is no part of the count: it reaches the thread's stack only through
// the options and defaults its modifiers read, and the nesting limit bounds
// those.
thread_local! {
    static IN_PROGRESS: Cell<usize> = const { Cell::new(0) };
    static REFUSED_UNWINDING: Cell<bool> = const { Cell::new(false) };
}

/// What [`Parser::resolve`](crate::Parser::resolve) unwinds with where it
/// refuses a resolution, before reading anything: [`MAX_REENTRY`] were in
/// progress on its thread. It unwinds into the host code that asked, a
/// modifier, a value's `Display` or a report handler, without running the
/// panic hook. That code may catch it, as it may any panic; one it lets pass
/// is contained as any panic of host code is, so the resolution around it
/// carries on. One refused while the thread already unwinds, from a `Drop`
/// say, starts no unwind of its own, since one leaving a `Drop` there would
/// abort the process: `resolve` answers the empty string to its caller
/// instead, and the host code the parser called that asked for it counts as
/// failed, as one that let the refusal pass does.
///
/// Its thread is the operating-system thread, so stackful fibers that share
/// one share its count, whether it unwinds, and the mark of a refusal made
/// while it does: on a host that runs them, a fiber where none is in
/// progress can be refused, and while one is suspended in a `Drop` that runs
/// as it unwinds, a refusal on any of them answers the empty string and can
/// fail another fiber's host call in place of the one that asked, short of
/// section 13's count per thread of execution.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct ReentryLimit;

impl fmt::Display for ReentryLimit {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            formatter,
            "a resolution was asked for while {MAX_REENTRY} were in progress on its thread"
        )
    }
}

/// One resolution in progress on this thread, counted while it lives. The
/// count is lowered on drop, so a panic unwinding past it leaves it right.
/// The count is this thread's, so the guard is not `Send`: what moves it to
/// another thread claims a `Send` it does not have.
pub(crate) struct InProgress {
    counted: bool,
    on_this_thread: PhantomData<*const ()>,
}

impl InProgress {
    /// A resolution in progress, or nothing where this thread already has
    /// [`MAX_REENTRY`] in progress. One asked for where none is never is
    /// refused.
    pub(crate) fn enter() -> Option<Self> {
        let entered = IN_PROGRESS.try_with(|count| {
            let held = count.get();

            if held >= MAX_REENTRY {
                return false;
            }

            count.set(held.saturating_add(1));

            true
        });

        match entered {
            Ok(true) => Some(Self {
                counted: true,
                on_this_thread: PhantomData,
            }),
            Ok(false) => None,
            // The count has no destructor, so it stays readable while the
            // thread tears its locals down, and a resolution there is counted
            // as any other. Should it ever be unreadable, a resolution goes
            // uncounted rather than refused.
            Err(_) => Some(Self {
                counted: false,
                on_this_thread: PhantomData,
            }),
        }
    }
}

impl Drop for InProgress {
    fn drop(&mut self) {
        if self.counted {
            let _ = IN_PROGRESS.try_with(|count| count.set(count.get().saturating_sub(1)));
        }
    }
}

/// Marks the host call in progress as failed: a resolution it asked for was
/// refused while the thread unwinds, and answered the empty string.
pub(crate) fn refused_while_unwinding() {
    let _ = REFUSED_UNWINDING.try_with(|refused| refused.set(true));
}

/// Runs `call`, answering nothing where it panicked, or where a resolution it
/// asked for was refused while the thread unwinds. A panic's payload, and an
/// answer given up, is dropped under the same containment, since dropping it
/// runs host code too: the flag is this call's until both are settled, so a
/// refusal made as they drop fails this call, which failed already, and never
/// the one around it. The flag is the OS thread's, so where fibers that share
/// it interleave, a refusal on one can fail another's call instead.
pub(crate) fn contained<T>(call: impl FnOnce() -> T) -> Option<T> {
    let outer = REFUSED_UNWINDING
        .try_with(|refused| refused.replace(false))
        .unwrap_or(false);
    let answer = match catch_unwind(AssertUnwindSafe(call)) {
        Ok(answer) if !REFUSED_UNWINDING.try_with(Cell::get).unwrap_or(false) => Some(answer),
        Ok(answer) => {
            discard(answer);

            None
        }
        Err(payload) => {
            discard(payload);

            None
        }
    };
    let _ = REFUSED_UNWINDING.try_with(|refused| refused.set(outer));

    answer
}

fn discard<T>(value: T) {
    let _ = catch_unwind(AssertUnwindSafe(move || drop(value)));
}

/// Calls host code: a report handler, a value's `Display`, a modifier. The
/// answer is nothing where the call panicked, a refused resolution it let
/// pass among them, or where a resolution it asked for was refused while the
/// thread unwinds.
///
/// No borrow of a walker's state is held across it.
pub(crate) fn host_call<T>(call: impl FnOnce() -> T) -> Option<T> {
    contained(call)
}

#[cfg(test)]
mod tests {
    use super::InProgress;

    // A guard that is `Send` gives the path two impls to pick from, and the
    // call does not compile: E0283, type annotations needed.
    #[test]
    fn a_resolution_in_progress_is_not_send() {
        trait AmbiguousIfSend<A> {
            fn some_item() {}
        }
        impl<T: ?Sized> AmbiguousIfSend<()> for T {}
        struct Sendable;
        impl<T: ?Sized + Send> AmbiguousIfSend<Sendable> for T {}

        let _ = <InProgress as AmbiguousIfSend<_>>::some_item;
    }
}
