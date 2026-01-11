use std::fs;
use std::path::Path;

use regex::Regex;

use crate::utils::error::{AppError, Result};

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
        if Path::new("/boot/grub/grub.cfg").exists()
            || Path::new("/etc/default/grub").exists()
        {
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
        let mut mirrors = Vec::new();

        for line in content.lines() {
            let trimmed = line.trim();

            // Skip empty lines
            if trimmed.is_empty() {
                continue;
            }

            // Check if it's a comment with a URL (disabled mirror)
            let (is_active, url) = if trimmed.starts_with('#') {
                let url_part = trimmed.trim_start_matches('#').trim();
                if url_part.starts_with("http://") || url_part.starts_with("https://")
                    || url_part.starts_with("ftp://")
                {
                    (false, url_part.to_string())
                } else {
                    continue; // Just a comment, skip
                }
            } else if trimmed.starts_with("http://")
                || trimmed.starts_with("https://")
                || trimmed.starts_with("ftp://")
            {
                (true, trimmed.to_string())
            } else {
                continue;
            };

            // Apply version filter if specified
            if let Some(filter) = version_filter {
                if !url.contains(filter) {
                    continue;
                }
            }

            let region = Self::extract_region(&url);
            mirrors.push(MirrorEntry {
                url,
                is_active,
                region,
            });
        }

        Ok(mirrors)
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
        let mut new_lines = Vec::new();

        for line in content.lines() {
            let trimmed = line.trim();

            if trimmed.is_empty() || trimmed.starts_with('#') && !trimmed.contains("://") {
                // Keep comments and empty lines as-is
                new_lines.push(line.to_string());
                continue;
            }

            // Extract URL from line (might be commented)
            let url = trimmed.trim_start_matches('#').trim();

            if url == mirror_url {
                // This is the mirror to activate
                new_lines.push(url.to_string());
            } else if trimmed.starts_with('#') {
                // Already commented, keep it
                new_lines.push(line.to_string());
            } else {
                // Active mirror that's not the target, comment it out
                new_lines.push(format!("# {}", trimmed));
            }
        }

        fs::write(mirrors_path, new_lines.join("\n") + "\n")?;
        Ok(())
    }

    /// Read a config file
    pub fn read_config(path: &str) -> Result<String> {
        fs::read_to_string(path).map_err(|e| {
            AppError::FileOperation(format!("Failed to read {}: {}", path, e))
        })
    }

    /// Write a config file
    pub fn write_config(path: &str, content: &str) -> Result<()> {
        fs::write(path, content).map_err(|e| {
            AppError::FileOperation(format!("Failed to write {}: {}", path, e))
        })
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
        let re = Regex::new(r"id:\d:initdefault:").map_err(|e| {
            AppError::Config(format!("Regex error: {}", e))
        })?;

        let new_content = re
            .replace(&content, format!("id:{}:initdefault:", runlevel).as_str())
            .to_string();

        fs::write(inittab_path, new_content)?;
        Ok(())
    }
}

/// Represents a mirror entry
#[derive(Debug, Clone)]
pub struct MirrorEntry {
    pub url: String,
    pub is_active: bool,
    pub region: String,
}

impl MirrorEntry {
    pub fn display(&self) -> String {
        let status = if self.is_active { "*" } else { " " };
        format!("[{}] {} ({})", status, self.url, self.region)
    }
}
