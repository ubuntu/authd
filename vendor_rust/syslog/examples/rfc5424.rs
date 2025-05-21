extern crate syslog;

use std::collections::BTreeMap;
use syslog::{Facility, Formatter5424};

fn main() {
    let formatter = Formatter5424 {
        facility: Facility::LOG_USER,
        hostname: None,
        process: "myprogram".into(),
        pid: 0,
    };

    match syslog::unix(formatter) {
        Err(e) => println!("impossible to connect to syslog: {:?}", e),
        Ok(mut writer) => {
            writer
                .err((1, BTreeMap::new(), "hello world"))
                .expect("could not write error message");
        }
    }
}
