use std::process::Stdio;
use tokio::process::Command;
use tokio::sync::mpsc;

/// Result of a command execution
#[derive(Debug, Clone)]
pub struct CommandResult {
    pub success: bool,
    pub stdout: String,
    pub stderr: String,
    pub exit_code: Option<i32>,
}

impl CommandResult {
    pub fn is_success(&self) -> bool {
        self.success
    }

    pub fn output(&self) -> &str {
        if self.stdout.is_empty() {
            &self.stderr
        } else {
            &self.stdout
        }
    }
}

/// Async command executor for running shell commands
pub struct CommandExecutor {
    /// Channel for sending command progress updates
    progress_tx: Option<mpsc::UnboundedSender<String>>,
}

impl CommandExecutor {
    pub fn new() -> Self {
        Self { progress_tx: None }
    }

    /// Create executor with progress channel
    pub fn with_progress(tx: mpsc::UnboundedSender<String>) -> Self {
        Self {
            progress_tx: Some(tx),
        }
    }

    /// Execute a command and return the result
    pub async fn execute(&self, cmd: &str, args: &[&str]) -> CommandResult {
        self.send_progress(format!("Running: {} {}", cmd, args.join(" ")));

        let output = Command::new(cmd)
            .args(args)
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .output()
            .await;

        match output {
            Ok(output) => {
                let stdout = String::from_utf8_lossy(&output.stdout).to_string();
                let stderr = String::from_utf8_lossy(&output.stderr).to_string();
                let success = output.status.success();

                if success {
                    self.send_progress("Command completed successfully".to_string());
                } else {
                    self.send_progress(format!("Command failed: {}", stderr));
                }

                CommandResult {
                    success,
                    stdout,
                    stderr,
                    exit_code: output.status.code(),
                }
            }
            Err(e) => {
                self.send_progress(format!("Failed to execute command: {}", e));
                CommandResult {
                    success: false,
                    stdout: String::new(),
                    stderr: e.to_string(),
                    exit_code: None,
                }
            }
        }
    }

    /// Execute a shell command (via /bin/sh -c)
    pub async fn execute_shell(&self, command: &str) -> CommandResult {
        self.execute("sh", &["-c", command]).await
    }

    /// Download a file using wget
    pub async fn download_file(&self, url: &str, output_path: &str) -> CommandResult {
        self.send_progress(format!("Downloading: {}", url));
        self.execute("wget", &["-O", output_path, url]).await
    }

    /// Install a Slackware package
    pub async fn installpkg(&self, package_path: &str) -> CommandResult {
        self.send_progress(format!("Installing package: {}", package_path));
        self.execute("installpkg", &[package_path]).await
    }

    /// Remove a Slackware package
    pub async fn removepkg(&self, package_name: &str) -> CommandResult {
        self.send_progress(format!("Removing package: {}", package_name));
        self.execute("removepkg", &[package_name]).await
    }

    /// Run slackpkg command
    pub async fn slackpkg(&self, args: &[&str]) -> CommandResult {
        self.send_progress(format!("Running slackpkg {}", args.join(" ")));
        self.execute("slackpkg", args).await
    }

    /// Run sbopkg command
    pub async fn sbopkg(&self, args: &[&str]) -> CommandResult {
        self.send_progress(format!("Running sbopkg {}", args.join(" ")));
        self.execute("sbopkg", args).await
    }

    /// Run sbotools commands
    pub async fn sboinstall(&self, package: &str) -> CommandResult {
        self.send_progress(format!("Installing SlackBuild: {}", package));
        self.execute("sboinstall", &["-j", package]).await
    }

    pub async fn sbofind(&self, query: &str) -> CommandResult {
        self.send_progress(format!("Searching SlackBuilds: {}", query));
        self.execute("sbofind", &[query]).await
    }

    pub async fn sboconfig(&self, args: &[&str]) -> CommandResult {
        self.send_progress(format!("Configuring sbotools: {}", args.join(" ")));
        self.execute("sboconfig", args).await
    }

    pub async fn sbosnap(&self, args: &[&str]) -> CommandResult {
        self.send_progress(format!("Running sbosnap {}", args.join(" ")));
        self.execute("sbosnap", args).await
    }

    /// Create a new user with useradd
    pub async fn useradd(
        &self,
        username: &str,
        groups: &[&str],
        shell: &str,
    ) -> CommandResult {
        let groups_str = groups.join(",");
        self.send_progress(format!("Creating user: {}", username));
        self.execute(
            "useradd",
            &[
                "-m",
                "-g",
                "users",
                "-G",
                &groups_str,
                "-s",
                shell,
                username,
            ],
        )
        .await
    }

    /// Set password for a user using chpasswd
    pub async fn set_password(&self, username: &str, password: &str) -> CommandResult {
        self.send_progress(format!("Setting password for: {}", username));
        let input = format!("{}:{}", username, password);

        let output = Command::new("chpasswd")
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn();

        match output {
            Ok(mut child) => {
                use tokio::io::AsyncWriteExt;
                if let Some(mut stdin) = child.stdin.take() {
                    let _ = stdin.write_all(input.as_bytes()).await;
                    let _ = stdin.shutdown().await;
                }

                match child.wait_with_output().await {
                    Ok(output) => CommandResult {
                        success: output.status.success(),
                        stdout: String::from_utf8_lossy(&output.stdout).to_string(),
                        stderr: String::from_utf8_lossy(&output.stderr).to_string(),
                        exit_code: output.status.code(),
                    },
                    Err(e) => CommandResult {
                        success: false,
                        stdout: String::new(),
                        stderr: e.to_string(),
                        exit_code: None,
                    },
                }
            }
            Err(e) => CommandResult {
                success: false,
                stdout: String::new(),
                stderr: e.to_string(),
                exit_code: None,
            },
        }
    }

    /// Run lilo bootloader
    pub async fn lilo(&self) -> CommandResult {
        self.send_progress("Running lilo bootloader update".to_string());
        self.execute("lilo", &[]).await
    }

    fn send_progress(&self, message: String) {
        if let Some(tx) = &self.progress_tx {
            let _ = tx.send(message);
        }
    }
}

impl Default for CommandExecutor {
    fn default() -> Self {
        Self::new()
    }
}
