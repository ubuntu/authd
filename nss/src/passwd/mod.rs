use crate::{error, REQUEST_TIMEOUT};
use libc::uid_t;
use libnss::interop::Response;
use libnss::passwd::{Passwd, PasswdHooks};
use tokio::runtime::Builder;
use tonic::Request;

use crate::client::{self, authd};
use authd::PasswdEntry;

pub struct AuthdPasswd;
impl PasswdHooks for AuthdPasswd {
    /// get_all_entries returns all passwd entries.
    fn get_all_entries() -> Response<Vec<Passwd>> {
        get_all_entries()
    }

    /// get_entry_by_uid returns the passwd entry for the given uid.
    fn get_entry_by_uid(uid: uid_t) -> Response<Passwd> {
        get_entry_by_uid(uid)
    }

    /// get_entry_by_name returns the passwd entry for the given name.
    fn get_entry_by_name(name: String) -> Response<Passwd> {
        get_entry_by_name(name)
    }
}

/// get_all_entries connects to the grpc server and asks for all passwd entries.
fn get_all_entries() -> Response<Vec<Passwd>> {
    let rt = match Builder::new_current_thread().enable_all().build() {
        Ok(rt) => rt,
        Err(e) => {
            error!("could not create runtime for NSS: {}", e);
            return Response::Unavail;
        }
    };

    rt.block_on(async {
        let mut client = match client::new_client().await {
            Ok(c) => c,
            Err(e) => {
                error!("could not connect to gRPC server: {}", e);
                return Response::Unavail;
            }
        };

        let mut req = Request::new(authd::Empty {});
        req.set_timeout(REQUEST_TIMEOUT);
        match client.get_passwd_entries(req).await {
            Ok(r) => Response::Success(passwd_entries_to_passwds(r.into_inner().entries)),
            Err(e) => {
                error!("error when listing passwd: {}", e.code());
                super::grpc_status_to_nss_response(e)
            }
        }
    })
}

/// get_entry_by_uid connects to the grpc server and asks for the passwd entry with the given uid.
fn get_entry_by_uid(uid: uid_t) -> Response<Passwd> {
    let rt = match Builder::new_current_thread().enable_all().build() {
        Ok(rt) => rt,
        Err(e) => {
            error!("could not create runtime for NSS: {}", e);
            return Response::Unavail;
        }
    };

    rt.block_on(async {
        let mut client = match client::new_client().await {
            Ok(c) => c,
            Err(e) => {
                error!("could not connect to gRPC server: {}", e);
                return Response::Unavail;
            }
        };

        let mut req = Request::new(authd::GetByIdRequest { id: uid });
        req.set_timeout(REQUEST_TIMEOUT);
        match client.get_passwd_by_uid(req).await {
            Ok(r) => Response::Success(passwd_entry_to_passwd(r.into_inner())),
            Err(e) => {
                error!("error when getting passwd by uid '{}': {}", uid, e.code());
                super::grpc_status_to_nss_response(e)
            }
        }
    })
}

/// get_entry_by_name connects to the grpc server and asks for the passwd entry with the given name.
fn get_entry_by_name(name: String) -> Response<Passwd> {
    let rt = match Builder::new_current_thread().enable_all().build() {
        Ok(rt) => rt,
        Err(e) => {
            error!("could not create runtime for NSS: {}", e);
            return Response::Unavail;
        }
    };

    rt.block_on(async {
        let mut client = match client::new_client().await {
            Ok(c) => c,
            Err(e) => {
                error!("could not connect to gRPC server: {}", e);
                return Response::Unavail;
            }
        };

        let mut req = Request::new(authd::GetPasswdByNameRequest {
            name: name.clone(),
            should_pre_check: should_pre_check(),
        });
        req.set_timeout(REQUEST_TIMEOUT);
        match client.get_passwd_by_name(req).await {
            Ok(r) => Response::Success(passwd_entry_to_passwd(r.into_inner())),
            Err(e) => {
                error!(
                    "error when getting passwd by name '{}': {}",
                    name,
                    e.code()
                );
                super::grpc_status_to_nss_response(e)
            }
        }
    })
}

/// passwd_entry_to_passwd converts a PasswdEntry to a libnss::Passwd.
fn passwd_entry_to_passwd(entry: PasswdEntry) -> Passwd {
    Passwd {
        name: entry.name,
        passwd: entry.passwd,
        uid: entry.uid,
        gid: entry.gid,
        gecos: entry.gecos,
        dir: entry.homedir,
        shell: entry.shell,
    }
}

/// passwd_entries_to_passwds converts a Vec<PasswdEntry> to a Vec<libnss::Passwd>.
fn passwd_entries_to_passwds(entries: Vec<PasswdEntry>) -> Vec<Passwd> {
    entries.into_iter().map(passwd_entry_to_passwd).collect()
}

/// should_pre_check returns true if the current process is a child of sshd.
#[allow(unreachable_code)] // This function body is overridden in integration tests, so we need to ignore the warning.
fn should_pre_check() -> bool {
    #[cfg(feature = "integration_tests")]
    return std::env::var("AUTHD_NSS_SHOULD_PRE_CHECK").is_ok();

    let ppid = std::os::unix::process::parent_id();
    let parent = procfs::process::Process::new(ppid as i32);
    if parent.is_err() {
        return false;
    }

    let cmds = parent.unwrap().cmdline();
    if cmds.is_err() {
        return false;
    }

    let cmds = cmds.unwrap();
    matches!(&cmds[0], s if s == "sshd")
}
