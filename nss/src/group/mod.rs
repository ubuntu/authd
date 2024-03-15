use crate::{error, REQUEST_TIMEOUT};
use libc::gid_t;
use libnss::group::{Group, GroupHooks};
use libnss::interop::Response;
use tokio::runtime::Builder;
use tonic::Request;

use crate::client::{self, authd};
use authd::GroupEntry;

pub struct AuthdGroup;
impl GroupHooks for AuthdGroup {
    /// get_all_entries returns all group entries.
    fn get_all_entries() -> Response<Vec<Group>> {
        get_all_entries()
    }

    /// get_entry_by_gid returns the group entry for the given gid.
    fn get_entry_by_gid(gid: gid_t) -> Response<Group> {
        get_entry_by_gid(gid)
    }

    /// get_entry_by_name returns the group entry for the given name.
    fn get_entry_by_name(name: String) -> Response<Group> {
        get_entry_by_name(name)
    }
}

/// get_all_entries connects to the grpc server and asks for all group entries.
fn get_all_entries() -> Response<Vec<Group>> {
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
        match client.get_group_entries(req).await {
            Ok(r) => Response::Success(group_entries_to_groups(r.into_inner().entries)),
            Err(e) => {
                error!("error when listing groups: {}", e.message());
                super::grpc_status_to_nss_response(e)
            }
        }
    })
}

/// get_entry_by_gid connects to the grpc server and asks for the group entry with the given gid.
fn get_entry_by_gid(gid: gid_t) -> Response<Group> {
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

        let mut req = Request::new(authd::GetByIdRequest { id: gid });
        req.set_timeout(REQUEST_TIMEOUT);
        match client.get_group_by_gid(req).await {
            Ok(r) => Response::Success(group_entry_to_group(r.into_inner())),
            Err(e) => {
                error!("error when getting group by gid: {}", e.message());
                super::grpc_status_to_nss_response(e)
            }
        }
    })
}

/// get_entry_by_name connects to the grpc server and asks for the group entry with the given name.
fn get_entry_by_name(name: String) -> Response<Group> {
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

        let mut req = Request::new(authd::GetGroupByNameRequest { name });
        req.set_timeout(REQUEST_TIMEOUT);
        match client.get_group_by_name(req).await {
            Ok(r) => Response::Success(group_entry_to_group(r.into_inner())),
            Err(e) => {
                error!("error when getting group by name: {}", e.message());
                super::grpc_status_to_nss_response(e)
            }
        }
    })
}

/// group_entry_to_group converts a GroupEntry to a libnss::Group.
fn group_entry_to_group(entry: GroupEntry) -> Group {
    Group {
        name: entry.name,
        passwd: entry.passwd,
        gid: entry.gid,
        members: entry.members,
    }
}

/// group_entries_to_groups converts a Vec<GroupEntry> to a Vec<libnss::Group>.
fn group_entries_to_groups(entries: Vec<GroupEntry>) -> Vec<Group> {
    entries.into_iter().map(group_entry_to_group).collect()
}
