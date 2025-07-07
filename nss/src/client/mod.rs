use authd::user_service_client::UserServiceClient;
use hyper_util::rt::TokioIo;
use std::error::Error;
use std::sync::OnceLock;
use tokio::net::UnixStream;
use tonic::transport::{Channel, Endpoint, Uri};
use tower::service_fn;

use crate::{info, CONNECTION_TIMEOUT};

pub mod authd {
    tonic::include_proto!("authd");
}

const AUTHD_PID_ENV_VAR: &str = "AUTHD_PID";

/// new_client creates a new client connection to the gRPC server or returns an active one.
pub async fn new_client() -> Result<UserServiceClient<Channel>, Box<dyn Error>> {
    info!("Connecting to authd on {}...", super::socket_path());

    // Cache for self-check result.
    static AUTHD_PROCESS_CHECK: OnceLock<bool> = OnceLock::new();

    let connector = service_fn(|_: Uri| async {
        let stream = UnixStream::connect(super::socket_path()).await?;

        if *AUTHD_PROCESS_CHECK.get_or_init(|| check_is_authd_process(&stream)) {
            info!("Module loaded by authd itself: ignoring the connection");

            return Err(std::io::Error::new(
                std::io::ErrorKind::Unsupported,
                "Ignoring connection from authd to authd itself",
            ));
        }

        Ok::<_, std::io::Error>(TokioIo::new(stream))
    });

    // The URL must have a valid format, even though we don't use it.
    let ch = Endpoint::try_from("https://not-used:404")?
        .connect_timeout(CONNECTION_TIMEOUT)
        .connect_with_connector(connector)
        .await?;

    Ok(UserServiceClient::new(ch))
}

fn check_is_authd_process(stream: &UnixStream) -> bool {
    // Check if we've been launched with a AUTHD_PID env variable set with
    // a numeric value. If these checks fail, we can just continue with the
    // connection as we were. As for sure the library has not been loaded
    // by authd.
    let Ok(authd_pid) = std::env::var(AUTHD_PID_ENV_VAR) else {
        return false;
    };
    info!(
        "authd module launched with {}={}",
        AUTHD_PID_ENV_VAR, authd_pid
    );
    let Ok(authd_pid_value) = authd_pid.parse::<u32>() else {
        return false;
    };

    let current_pid = std::process::id();
    info!("current PID is {}", current_pid);
    if current_pid != authd_pid_value {
        return false;
    }

    // Get the peer credentials, and check if the server PIDs matches the
    // AUTHD_PID, an if it does, we can avoid any connection since we're
    // sure that we have been loaded by authd (and not by another crafted
    // client to act like it, to ignore the authd module)
    let Ok(peer_cred) = stream.peer_cred() else {
        return false;
    };
    let Some(peer_pid) = peer_cred.pid() else {
        return false;
    };

    info!(
        "authd socket is provided by PID {} (expecting {})",
        peer_pid, authd_pid
    );
    if authd_pid_value != peer_pid.try_into().unwrap() {
        return false;
    }

    return true;
}
