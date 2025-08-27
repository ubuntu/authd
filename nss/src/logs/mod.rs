use log::{LevelFilter, Metadata};
use simple_logger::SimpleLogger;
use std::env;
use syslog::{BasicLogger, Facility, Formatter3164};

#[macro_export]
macro_rules! info {
    ($($arg:tt)*) => {
        let log_prefix = "authd:";
        log::info!("{} {}", log_prefix, format_args!($($arg)*));
    }
}

/// init_logger initialize the global logger with a default level set to info. This function is only
/// required to be called once and is a no-op on subsequent calls.
///
/// The log level can be set to info by setting the environment variable AUTHD_NSS_INFO.
pub fn init_logger() {
    if log::logger().enabled(&Metadata::builder().build()) {
        return;
    }

    let mut level = LevelFilter::Error;
    if let Ok(target) = env::var("AUTHD_NSS_INFO") {
        level = LevelFilter::Info;
        match target {
            s if s == *"stderr" => init_stderr_logger(level),
            _ => init_sys_logger(level),
        }
    } else {
        init_sys_logger(level);
    }

    info!("Log level set to {:?}", level);
}

/// init_sys_logger initializes a global log that prints messages to the system logs.
fn init_sys_logger(log_level: LevelFilter) {
    // Derive the process name from current_exe(), fall back to a sensible default.
    let process_name = std::env::current_exe()
        .ok()
        .and_then(|p| p.file_name().map(|s| s.to_string_lossy().into_owned()))
        .filter(|s| !s.is_empty())
        .unwrap_or_else(|| "nss-authd".to_string());

    let formatter = Formatter3164 {
        facility: Facility::LOG_USER,
        hostname: None,
        process: process_name,
        pid: std::process::id(),
    };

    let logger = match syslog::unix(formatter) {
        Err(err) => {
            println!("cannot connect to syslog: {err:?}");
            return;
        }
        Ok(l) => l,
    };

    if let Err(err) = log::set_boxed_logger(Box::new(BasicLogger::new(logger)))
        .map(|()| log::set_max_level(log_level))
    {
        eprintln!("cannot set log level: {err:?}");
        return;
    };

    info!("Log output set to syslog");
}

/// init_stderr_logger initializes a global log that prints the messages to stderr.
fn init_stderr_logger(log_level: LevelFilter) {
    SimpleLogger::new().with_level(log_level).init().unwrap();
    info!("Log output set to stderr");
}
