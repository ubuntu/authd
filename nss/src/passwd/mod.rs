use crate::error;
use libc::uid_t;
use libnss::interop::Response;
use libnss::passwd::{Passwd, PasswdHooks};
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
    super::RT.block_on(async {
        let mut client = match client::new_client().await {
            Ok(c) => c,
            Err(e) => {
                error!("could not connect to gRPC server: {}", e);
                return Response::Unavail;
            }
        };

        let req = Request::new(authd::Empty {});
        match client.get_passwd_entries(req).await {
            Ok(r) => Response::Success(passwd_entries_to_passwds(r.into_inner().entries)),
            Err(e) => {
                error!("error when listing passwd: {}", e.message());
                super::grpc_status_to_nss_response(e)
            }
        }
    })
}

/// get_entry_by_uid connects to the grpc server and asks for the passwd entry with the given uid.
fn get_entry_by_uid(uid: uid_t) -> Response<Passwd> {
    super::RT.block_on(async {
        let mut client = match client::new_client().await {
            Ok(c) => c,
            Err(e) => {
                error!("could not connect to gRPC server: {}", e);
                return Response::Unavail;
            }
        };

        let req = Request::new(authd::GetByIdRequest { id: uid });
        match client.get_passwd_by_uid(req).await {
            Ok(r) => Response::Success(passwd_entry_to_passwd(r.into_inner())),
            Err(e) => {
                error!("error when getting passwd by uid: {}", e.message());
                super::grpc_status_to_nss_response(e)
            }
        }
    })
}

/// get_entry_by_name connects to the grpc server and asks for the passwd entry with the given name.
fn get_entry_by_name(name: String) -> Response<Passwd> {
    super::RT.block_on(async {
        let mut client = match client::new_client().await {
            Ok(c) => c,
            Err(e) => {
                error!("could not connect to gRPC server: {}", e);
                return Response::Unavail;
            }
        };

        let req = Request::new(authd::GetByNameRequest { name });
        match client.get_passwd_by_name(req).await {
            Ok(r) => Response::Success(passwd_entry_to_passwd(r.into_inner())),
            Err(e) => {
                error!("error when getting passwd by name: {}", e.message());
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
