use crate::error;
use libnss::interop::Response;
use libnss::shadow::{Shadow, ShadowHooks};
use tonic::Request;

use crate::client::{self, authd};
use authd::ShadowEntry;

pub struct AuthdShadow;

impl ShadowHooks for AuthdShadow {
    /// get_all_entries returns all shadow entries.
    fn get_all_entries() -> Response<Vec<Shadow>> {
        get_all_entries()
    }

    /// get_entry_by_name returns the shadow entry for the given name.
    fn get_entry_by_name(name: String) -> Response<Shadow> {
        get_entry_by_name(name)
    }
}

/// get_all_entries connects to the grpc server and asks for all shadow entries.
fn get_all_entries() -> Response<Vec<Shadow>> {
    super::RT.block_on(async {
        let mut client = match client::new_client().await {
            Ok(c) => c,
            Err(e) => {
                error!("could not connect to gRPC server: {}", e);
                return Response::Unavail;
            }
        };

        let req = Request::new(authd::Empty {});
        match client.get_shadow_entries(req).await {
            Ok(r) => Response::Success(shadow_entries_to_shadows(r.into_inner().entries)),
            Err(e) => {
                error!("error when listing shadow: {}", e.message());
                super::grpc_status_to_nss_response(e)
            }
        }
    })
}

/// get_entry_by_name connects to the grpc server and asks for the shadow entry with the given name.
fn get_entry_by_name(name: String) -> Response<Shadow> {
    super::RT.block_on(async {
        let mut client = match client::new_client().await {
            Ok(c) => c,
            Err(e) => {
                error!("could not connect to gRPC server: {}", e);
                return Response::Unavail;
            }
        };

        let req = Request::new(authd::GetByNameRequest { name });
        match client.get_shadow_by_name(req).await {
            Ok(r) => Response::Success(shadow_entry_to_shadow(r.into_inner())),
            Err(e) => {
                error!("error when getting shadow by name: {}", e.message());
                super::grpc_status_to_nss_response(e)
            }
        }
    })
}

/// shadow_entries_to_shadows converts a vector of shadow entries to a vector of shadows.
fn shadow_entry_to_shadow(entry: ShadowEntry) -> Shadow {
    Shadow {
        name: entry.name,
        passwd: entry.passwd,
        last_change: entry.last_change as isize,
        change_min_days: entry.change_min_days as isize,
        change_max_days: entry.change_max_days as isize,
        change_warn_days: entry.change_warn_days as isize,
        change_inactive_days: entry.change_inactive_days as isize,
        expire_date: entry.expire_date as isize,
        reserved: usize::MAX,
    }
}

/// shadow_entries_to_shadows converts a vector of shadow entries to a vector of shadows.
fn shadow_entries_to_shadows(entries: Vec<ShadowEntry>) -> Vec<Shadow> {
    entries.into_iter().map(shadow_entry_to_shadow).collect()
}
