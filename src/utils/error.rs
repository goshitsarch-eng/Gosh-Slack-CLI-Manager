use thiserror::Error;

#[derive(Error, Debug)]
pub enum AppError {
    #[error("This application must be run as root")]
    NotRoot,

    #[error("Failed to detect Slackware version: {0}")]
    VersionDetection(String),

    #[error("Command execution failed: {0}")]
    CommandFailed(String),

    #[error("File operation failed: {0}")]
    FileOperation(String),

    #[error("Configuration error: {0}")]
    Config(String),

    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),

    #[error("User cancelled operation")]
    UserCancelled,
}

pub type Result<T> = std::result::Result<T, AppError>;
