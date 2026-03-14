use nix::unistd::Uid;

/// Check if running as root, returns bool instead of Result
pub fn is_root() -> bool {
    Uid::effective().is_root()
}
