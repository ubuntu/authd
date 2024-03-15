use authd::nss_client::NssClient;
use std::error::Error;
use tokio::net::UnixStream;
use tonic::transport::{Channel, Endpoint, Uri};
use tower::service_fn;

use crate::{debug, REQUEST_TIMEOUT};

pub mod authd {
    tonic::include_proto!("authd");
}

/// new_client creates a new client connection to the gRPC server or returns an active one.
pub async fn new_client() -> Result<NssClient<Channel>, Box<dyn Error>> {
    // We need to skip NSS lookups performed by dbus through systemd, otherwise
    // we could end up in a deadlock due to lookups happening while the authd
    // daemon is starting up.
    // This variable is set by systemd specifically for dbus.service to avoid a
    // similar issue with nss-systemd - we can repurpose it for our case.
    // ref: https://github.com/systemd/systemd/pull/22552
    if std::env::var("SYSTEMD_NSS_DYNAMIC_BYPASS").is_ok() {
        return Err("NSS lookup performed through systemd, skipping...".into());
    }

    debug!("Connecting to authd on {}...", super::socket_path());

    // The URL must have a valid format, even though we don't use it.
    let ch = Endpoint::try_from("https://not-used:404")?
        .connect_timeout(REQUEST_TIMEOUT)
        .connect_with_connector(service_fn(|_: Uri| {
            UnixStream::connect(super::socket_path())
        }))
        .await?;

    Ok(NssClient::new(ch))
}
