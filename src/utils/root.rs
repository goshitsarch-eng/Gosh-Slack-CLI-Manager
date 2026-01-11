use nix::unistd::Uid;

use super::error::{AppError, Result};

/// Check if the current process is running as root (UID 0)
pub fn check_root() -> Result<()> {
    if Uid::effective().is_root() {
        Ok(())
    } else {
        Err(AppError::NotRoot)
    }
}

/// Check if running as root, returns bool instead of Result
pub fn is_root() -> bool {
    Uid::effective().is_root()
}
