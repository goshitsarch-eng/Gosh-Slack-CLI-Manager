use std::fs;
use std::path::Path;

use regex::Regex;

use crate::utils::error::{AppError, Result};
use crate::utils::fs::atomic_write;

/// Detected bootloader type
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Bootloader {
    Lilo,
    Grub,
    Unknown,
}

impl Bootloader {
    /// Detect which bootloader is installed on the system
    pub fn detect() -> Self {
        // Check for LILO first (Slackware default)
        if Path::new("/etc/lilo.conf").exists() {
            return Bootloader::Lilo;
        }

        // Check for GRUB
        if Path::new("/boot/grub/grub.cfg").exists() || Path::new("/etc/default/grub").exists() {
            return Bootloader::Grub;
        }

        Bootloader::Unknown
    }

    /// Get display name for the bootloader
    pub fn name(&self) -> &'static str {
        match self {
            Bootloader::Lilo => "LILO",
            Bootloader::Grub => "GRUB",
            Bootloader::Unknown => "Unknown",
        }
    }
}

/// Slackware configuration management
pub struct SlackwareConfig;

impl SlackwareConfig {
    /// Parse mirrors from /etc/slackpkg/mirrors
    pub fn parse_mirrors(version_filter: Option<&str>) -> Result<Vec<MirrorEntry>> {
        let mirrors_path = Path::new("/etc/slackpkg/mirrors");

        if !mirrors_path.exists() {
            return Err(AppError::FileOperation(
                "Mirrors file not found at /etc/slackpkg/mirrors".to_string(),
            ));
        }

        let content = fs::read_to_string(mirrors_path)?;
        Ok(Self::parse_mirrors_from_content(&content, version_filter))
    }

    /// Extract region/country from mirror URL
    fn extract_region(url: &str) -> String {
        // Try to extract country code from URL
        let re = Regex::new(r"mirrors\.(\w+)\.|\.(\w{2})/|/(\w{2})/").ok();

        if let Some(re) = re {
            if let Some(caps) = re.captures(url) {
                for i in 1..=3 {
                    if let Some(m) = caps.get(i) {
                        return m.as_str().to_uppercase();
                    }
                }
            }
        }

        // Try common patterns
        if url.contains("kernel.org") {
            return "US".to_string();
        }
        if url.contains("osuosl") {
            return "US".to_string();
        }
        if url.contains("ukfast") {
            return "UK".to_string();
        }

        "Unknown".to_string()
    }

    /// Set the active mirror in /etc/slackpkg/mirrors
    pub fn set_active_mirror(mirror_url: &str) -> Result<()> {
        let mirrors_path = Path::new("/etc/slackpkg/mirrors");

        if !mirrors_path.exists() {
            return Err(AppError::FileOperation(
                "Mirrors file not found".to_string(),
            ));
        }

        let content = fs::read_to_string(mirrors_path)?;
        let new_content = Self::rewrite_mirrors_content(&content, mirror_url)?;

        let backup_path = mirrors_path.with_extension("bak");
        let _ = fs::copy(mirrors_path, &backup_path);
        atomic_write(mirrors_path, &new_content)?;
        Ok(())
    }

    /// Modify /etc/inittab to change default runlevel
    pub fn set_default_runlevel(runlevel: u8) -> Result<()> {
        let inittab_path = "/etc/inittab";

        if !Path::new(inittab_path).exists() {
            return Err(AppError::FileOperation(
                "/etc/inittab not found".to_string(),
            ));
        }

        let content = fs::read_to_string(inittab_path)?;
        let re = Regex::new(r"id:\d:initdefault:")
            .map_err(|e| AppError::Config(format!("Regex error: {}", e)))?;

        let new_content = re
            .replace(&content, format!("id:{}:initdefault:", runlevel).as_str())
            .to_string();

        atomic_write(inittab_path, &new_content)?;
        Ok(())
    }

    fn parse_mirrors_from_content(content: &str, version_filter: Option<&str>) -> Vec<MirrorEntry> {
        let mut mirrors = Vec::new();

        for line in content.lines() {
            let trimmed = line.trim();

            if trimmed.is_empty() {
                continue;
            }

            let (is_active, url) = if trimmed.starts_with('#') {
                let url_part = trimmed.trim_start_matches('#').trim();
                if url_part.starts_with("http://")
                    || url_part.starts_with("https://")
                    || url_part.starts_with("ftp://")
                {
                    (false, url_part.to_string())
                } else {
                    continue;
                }
            } else if trimmed.starts_with("http://")
                || trimmed.starts_with("https://")
                || trimmed.starts_with("ftp://")
            {
                (true, trimmed.to_string())
            } else {
                continue;
            };

            if let Some(filter) = version_filter {
                if !url.contains(filter) {
                    continue;
                }
            }

            mirrors.push(MirrorEntry {
                region: Self::extract_region(&url),
                url,
                is_active,
            });
        }

        mirrors
    }

    fn rewrite_mirrors_content(content: &str, mirror_url: &str) -> Result<String> {
        let original_lines: Vec<&str> = content.lines().collect();
        let exists = original_lines.iter().any(|line| {
            let trimmed = line.trim();
            let url = trimmed.trim_start_matches('#').trim();
            url == mirror_url
        });

        if !exists {
            return Err(AppError::Config(format!(
                "Mirror '{}' was not found in mirrors file",
                mirror_url
            )));
        }

        let mut new_lines = Vec::new();
        let mut active_mirrors = 0usize;

        for line in &original_lines {
            let trimmed = line.trim();

            if trimmed.is_empty() || trimmed.starts_with('#') && !trimmed.contains("://") {
                new_lines.push(line.to_string());
                continue;
            }

            let url = trimmed.trim_start_matches('#').trim();

            if url == mirror_url {
                new_lines.push(url.to_string());
                active_mirrors += 1;
            } else if trimmed.starts_with('#') {
                new_lines.push((*line).to_string());
            } else {
                new_lines.push(format!("# {}", trimmed));
            }
        }

        if active_mirrors != 1 {
            return Err(AppError::Config(format!(
                "Mirror rewrite left {} active mirrors; expected exactly one",
                active_mirrors
            )));
        }

        Ok(new_lines.join("\n") + "\n")
    }
}

/// Represents a mirror entry
#[derive(Debug, Clone)]
pub struct MirrorEntry {
    pub url: String,
    pub is_active: bool,
    pub region: String,
}

#[cfg(test)]
mod tests {
    use super::SlackwareConfig;

    #[test]
    fn parse_mirrors_filters_by_version_and_tracks_active_state() {
        let content = r#"
# comment
https://mirror1.example/slackware64-15.0/
# https://mirror2.example/slackware64-15.0/
https://mirror3.example/slackware64-current/
"#;

        let mirrors =
            SlackwareConfig::parse_mirrors_from_content(content, Some("slackware64-15.0"));

        assert_eq!(mirrors.len(), 2);
        assert!(mirrors[0].is_active);
        assert!(!mirrors[1].is_active);
        assert_eq!(mirrors[0].url, "https://mirror1.example/slackware64-15.0/");
    }

    #[test]
    fn rewrite_mirrors_content_enables_exactly_one_mirror() {
        let content = r#"
# Slackware mirrors
https://mirror1.example/slackware64-15.0/
# https://mirror2.example/slackware64-15.0/
"#;

        let rewritten = SlackwareConfig::rewrite_mirrors_content(
            content,
            "https://mirror2.example/slackware64-15.0/",
        )
        .unwrap();

        assert!(rewritten.contains("# https://mirror1.example/slackware64-15.0/"));
        assert!(rewritten.contains("https://mirror2.example/slackware64-15.0/"));
        assert_eq!(
            rewritten
                .lines()
                .filter(|line| line.starts_with("https://"))
                .count(),
            1
        );
    }
}
