#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    clippy::indexing_slicing,
    clippy::arithmetic_side_effects
)]
// A counting allocator is the one way to see allocations from outside the
// crate, and installing one takes an unsafe impl.
#![allow(unsafe_code)]

use std::alloc::{GlobalAlloc, Layout, System};
use std::cell::Cell;

use curly_message_parser::Object;

struct Counting;

thread_local! {
    static ALLOCATIONS: Cell<usize> = const { Cell::new(0) };
}

// SAFETY: every call is forwarded to the system allocator unchanged; the count
// is a thread-local `Cell` of a const initializer, which allocates nothing.
unsafe impl GlobalAlloc for Counting {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        let _ = ALLOCATIONS.try_with(|count| count.set(count.get() + 1));
        // SAFETY: the caller's contract is passed on as it is.
        unsafe { System.alloc(layout) }
    }

    unsafe fn dealloc(&self, ptr: *mut u8, layout: Layout) {
        // SAFETY: the caller's contract is passed on as it is.
        unsafe { System.dealloc(ptr, layout) }
    }

    unsafe fn realloc(&self, ptr: *mut u8, layout: Layout, new_size: usize) -> *mut u8 {
        let _ = ALLOCATIONS.try_with(|count| count.set(count.get() + 1));
        // SAFETY: the caller's contract is passed on as it is.
        unsafe { System.realloc(ptr, layout, new_size) }
    }
}

#[global_allocator]
static GLOBAL: Counting = Counting;

fn allocations<T>(run: impl FnOnce() -> T) -> (usize, T) {
    let before = ALLOCATIONS.with(Cell::get);
    let output = run();

    (ALLOCATIONS.with(Cell::get) - before, output)
}

#[test]
fn an_object_of_n_keys_costs_n_plus_two_allocations() {
    let keys: Vec<String> = (0..1000).map(|n| format!("key{n}")).collect();

    for size in [1, 8, 9, 100, 1000] {
        let entries: Vec<(&str, i32)> = keys
            .iter()
            .take(size)
            .map(|key| (key.as_str(), 0))
            .collect();
        let (count, object) = allocations(|| entries.iter().copied().collect::<Object>());
        let expected = if size > 8 { size + 2 } else { size + 1 };

        assert_eq!(count, expected, "{size} keys");
        assert_eq!(object.len(), size);
    }
}
