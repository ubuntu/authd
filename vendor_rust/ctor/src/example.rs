//! This example demonstrates the various types of ctor/dtor in an executable
//! context.

#![cfg_attr(feature = "used_linker", feature(used_with_arg))]

use ctor::{ctor, dtor};
use libc_print::*;
use std::collections::HashMap;

#[ctor]
/// This is an immutable static, evaluated at init time
static STATIC_CTOR: HashMap<u32, &'static str> = unsafe {
    let mut m = HashMap::new();
    _ = m.insert(0, "foo");
    _ = m.insert(1, "bar");
    _ = m.insert(2, "baz");
    libc_eprintln!("STATIC_CTOR");
    m
};

#[ctor]
#[allow(unsafe_code)]
unsafe fn ctor() {
    libc_eprintln!("ctor");
}

#[ctor]
#[allow(unsafe_code)]
unsafe fn ctor_unsafe() {
    libc_eprintln!("ctor_unsafe");
}

#[dtor]
#[allow(unsafe_code)]
unsafe fn dtor() {
    libc_eprintln!("dtor");
}

#[dtor]
#[allow(unsafe_code)]
unsafe fn dtor_unsafe() {
    libc_eprintln!("dtor_unsafe");
}

/// A module with a static ctor/dtor
pub mod module {
    use ctor::*;
    use libc_print::*;

    #[ctor]
    pub(crate) static STATIC_CTOR: u8 = unsafe {
        libc_eprintln!("module::STATIC_CTOR");
        42
    };

    #[dtor]
    #[allow(unsafe_code)]
    unsafe fn dtor_module() {
        libc_eprintln!("module::dtor_module");
    }
}

/// Executable main which demonstrates the various types of ctor/dtor.
pub fn main() {
    use libc_print::*;
    libc_eprintln!("main!");
    libc_eprintln!("STATIC_CTOR = {:?}", *STATIC_CTOR);
    libc_eprintln!("module::STATIC_CTOR = {:?}", *module::STATIC_CTOR);
}
