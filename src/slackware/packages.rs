use super::commands::CommandExecutor;

/// Package information from SlackBuilds
#[derive(Debug, Clone)]
pub struct PackageInfo {
    pub name: String,
    pub category: String,
    pub description: String,
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
    pub async fn search(&self, query: &str) -> Result<Vec<PackageInfo>, String> {
        let result = self.executor.sbofind(query).await;

        if !result.success {
            return Err(if result.stderr.is_empty() {
                "sbofind did not return any output".to_string()
            } else {
                result.stderr
            });
        }

        Ok(self.parse_sbofind_output(&result.stdout))
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
            });
        }

        packages
    }
}

impl Default for PackageManager {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::PackageManager;

    #[test]
    fn parse_sbofind_output_parses_multiple_entries() {
        let manager = PackageManager::new();
        let output = r#"
SBo:    desktop/foo
Path:   /var/lib/sbopkg/SBo/15.0/desktop/foo
info:   Foo desktop utility

SBo:    system/bar
Path:   /var/lib/sbopkg/SBo/15.0/system/bar
info:   Bar system utility
"#;

        let packages = manager.parse_sbofind_output(output);

        assert_eq!(packages.len(), 2);
        assert_eq!(packages[0].name, "foo");
        assert_eq!(packages[0].category, "desktop");
        assert_eq!(packages[0].description, "Foo desktop utility");
        assert_eq!(packages[1].name, "bar");
        assert_eq!(packages[1].category, "system");
    }

    #[test]
    fn parse_sbofind_output_keeps_last_entry_without_trailing_blank_line() {
        let manager = PackageManager::new();
        let output = r#"
SBo:    network/curlie
Path:   /var/lib/sbopkg/SBo/15.0/network/curlie
info:   Curl wrapper
"#;

        let packages = manager.parse_sbofind_output(output);

        assert_eq!(packages.len(), 1);
        assert_eq!(packages[0].name, "curlie");
        assert_eq!(packages[0].description, "Curl wrapper");
    }
}
