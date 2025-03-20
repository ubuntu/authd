#[cfg(feature = "set")]
use std::ffi::OsStr;
use std::ffi::OsString;
use std::io;
#[cfg(feature = "set")]
use std::os::windows::ffi::OsStrExt;
use std::os::windows::ffi::OsStringExt;

use windows::core::PWSTR;
use windows::Win32::System::SystemInformation::{
    ComputerNamePhysicalDnsHostname, GetComputerNameExW,
};

pub fn get() -> io::Result<OsString> {
    let mut size = 0;
    unsafe {
        // Don't care much about the result here,
        // it is guaranteed to return an error,
        // since we passed the NULL pointer as a buffer
        let result = GetComputerNameExW(ComputerNamePhysicalDnsHostname, PWSTR::null(), &mut size);
        debug_assert!(result.is_err());
    };

    let mut buffer = Vec::with_capacity(size as usize);

    let result = unsafe {
        GetComputerNameExW(
            ComputerNamePhysicalDnsHostname,
            PWSTR::from_raw(buffer.as_mut_ptr()),
            &mut size,
        )
    };

    match result {
        Ok(_) => {
            unsafe {
                buffer.set_len(size as usize);
            }

            Ok(OsString::from_wide(&buffer))
        }
        Err(e) => Err(io::Error::from_raw_os_error(e.code().0)),
    }
}

#[cfg(feature = "set")]
pub fn set(hostname: &OsStr) -> io::Result<()> {
    use windows::core::PCWSTR;
    use windows::Win32::System::SystemInformation::SetComputerNameExW;

    let mut buffer = hostname.encode_wide().collect::<Vec<_>>();
    buffer.push(0x00); // Appending the null terminator

    let result = unsafe {
        SetComputerNameExW(
            ComputerNamePhysicalDnsHostname,
            PCWSTR::from_raw(buffer.as_ptr()),
        )
    };

    result.map_err(|e| io::Error::from_raw_os_error(e.code().0))
}
