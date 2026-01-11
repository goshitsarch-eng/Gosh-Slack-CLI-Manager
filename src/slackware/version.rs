use std::fs;
use std::path::Path;

use crate::utils::error::{AppError, Result};

/// Represents a Slackware version
#[derive(Debug, Clone, PartialEq)]
pub enum SlackwareVersion {
    Current,
    V15_0,
    V14_2,
    V14_1,
    Unknown(String),
}

impl SlackwareVersion {
    /// Parse version string into SlackwareVersion enum
    pub fn from_string(version: &str) -> Self {
        let version = version.trim().to_lowercase();

        if version.contains("current") {
            SlackwareVersion::Current
        } else if version.contains("15.0") {
            SlackwareVersion::V15_0
        } else if version.contains("14.2") {
            SlackwareVersion::V14_2
        } else if version.contains("14.1") {
            SlackwareVersion::V14_1
        } else {
            SlackwareVersion::Unknown(version)
        }
    }

    /// Get the mirror path suffix for this version
    pub fn mirror_path(&self) -> &str {
        match self {
            SlackwareVersion::Current => "slackware64-current",
            SlackwareVersion::V15_0 => "slackware64-15.0",
            SlackwareVersion::V14_2 => "slackware64-14.2",
            SlackwareVersion::V14_1 => "slackware64-14.1",
            SlackwareVersion::Unknown(_) => "slackware64-current",
        }
    }

    /// Get display name for the version
    pub fn display_name(&self) -> String {
        match self {
            SlackwareVersion::Current => "Slackware64 Current".to_string(),
            SlackwareVersion::V15_0 => "Slackware64 15.0".to_string(),
            SlackwareVersion::V14_2 => "Slackware64 14.2".to_string(),
            SlackwareVersion::V14_1 => "Slackware64 14.1".to_string(),
            SlackwareVersion::Unknown(v) => format!("Slackware ({})", v),
        }
    }

    /// Get all supported versions
    pub fn all_versions() -> Vec<SlackwareVersion> {
        vec![
            SlackwareVersion::Current,
            SlackwareVersion::V15_0,
            SlackwareVersion::V14_2,
            SlackwareVersion::V14_1,
        ]
    }
}

impl std::fmt::Display for SlackwareVersion {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.display_name())
    }
}

/// Detect the installed Slackware version by reading /etc/slackware-version
pub fn detect_version() -> Result<SlackwareVersion> {
    let version_file = Path::new("/etc/slackware-version");

    if !version_file.exists() {
        return Err(AppError::VersionDetection(
            "File /etc/slackware-version not found. Is this a Slackware system?".to_string(),
        ));
    }

    let content = fs::read_to_string(version_file).map_err(|e| {
        AppError::VersionDetection(format!("Failed to read /etc/slackware-version: {}", e))
    })?;

    Ok(SlackwareVersion::from_string(&content))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_version_parsing() {
        assert_eq!(
            SlackwareVersion::from_string("Slackware 15.0"),
            SlackwareVersion::V15_0
        );
        assert_eq!(
            SlackwareVersion::from_string("Slackware 14.2"),
            SlackwareVersion::V14_2
        );
        assert_eq!(
            SlackwareVersion::from_string("Slackware Linux -current"),
            SlackwareVersion::Current
        );
    }

    #[test]
    fn test_mirror_path() {
        assert_eq!(SlackwareVersion::V15_0.mirror_path(), "slackware64-15.0");
        assert_eq!(
            SlackwareVersion::Current.mirror_path(),
            "slackware64-current"
        );
    }
}
