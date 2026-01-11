use super::commands::{CommandExecutor, CommandResult};

/// Package information from SlackBuilds
#[derive(Debug, Clone)]
pub struct PackageInfo {
    pub name: String,
    pub category: String,
    pub description: String,
    pub version: Option<String>,
    pub installed: bool,
}

/// Package manager for SlackBuilds.org packages
pub struct PackageManager {
    executor: CommandExecutor,
}

impl PackageManager {
    pub fn new() -> Self {
        Self {
            executor: CommandExecutor::new(),
        }
    }

    /// Search for packages using sbofind
    pub async fn search(&self, query: &str) -> Vec<PackageInfo> {
        let result = self.executor.sbofind(query).await;

        if !result.success {
            return Vec::new();
        }

        self.parse_sbofind_output(&result.stdout)
    }

    /// Parse sbofind output into PackageInfo structs
    fn parse_sbofind_output(&self, output: &str) -> Vec<PackageInfo> {
        let mut packages = Vec::new();
        let mut current_name = String::new();
        let mut current_category = String::new();
        let mut current_description = String::new();

        for line in output.lines() {
            let line = line.trim();

            if line.is_empty() {
                if !current_name.is_empty() {
                    packages.push(PackageInfo {
                        name: current_name.clone(),
                        category: current_category.clone(),
                        description: current_description.clone(),
                        version: None,
                        installed: false,
                    });
                    current_name.clear();
                    current_category.clear();
                    current_description.clear();
                }
                continue;
            }

            // sbofind output format:
            // SBo:    category/package
            // Path:   /var/lib/sbopkg/...
            // info:   Description line
            if line.starts_with("SBo:") {
                let path = line.trim_start_matches("SBo:").trim();
                if let Some(idx) = path.find('/') {
                    current_category = path[..idx].to_string();
                    current_name = path[idx + 1..].to_string();
                } else {
                    current_name = path.to_string();
                }
            } else if line.starts_with("info:") {
                current_description = line.trim_start_matches("info:").trim().to_string();
            }
        }

        // Don't forget the last entry
        if !current_name.is_empty() {
            packages.push(PackageInfo {
                name: current_name,
                category: current_category,
                description: current_description,
                version: None,
                installed: false,
            });
        }

        packages
    }

    /// Install a package using sboinstall
    pub async fn install(&self, package_name: &str) -> CommandResult {
        self.executor.sboinstall(package_name).await
    }

    /// Get info about a specific package
    pub async fn get_info(&self, package_name: &str) -> Option<String> {
        let result = self.executor.sbofind(package_name).await;

        if result.success {
            Some(result.stdout)
        } else {
            None
        }
    }
}

impl Default for PackageManager {
    fn default() -> Self {
        Self::new()
    }
}
