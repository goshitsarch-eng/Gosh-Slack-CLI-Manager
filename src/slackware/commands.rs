use std::process::Stdio;

use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::Command;
use tokio::sync::mpsc;

/// Result of a command execution
#[derive(Debug, Clone)]
pub struct CommandResult {
    pub success: bool,
    pub stdout: String,
    pub stderr: String,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum StreamSource {
    Stdout,
    Stderr,
}

/// Async command executor for running shell commands
#[derive(Clone)]
pub struct CommandExecutor;

impl CommandExecutor {
    pub fn new() -> Self {
        Self
    }

    async fn read_stream<R>(
        reader: R,
        tx: mpsc::UnboundedSender<(StreamSource, String)>,
        source: StreamSource,
    ) -> std::io::Result<()>
    where
        R: tokio::io::AsyncRead + Unpin,
    {
        let mut lines = BufReader::new(reader).lines();
        while let Some(line) = lines.next_line().await? {
            let _ = tx.send((source, line));
        }

        Ok(())
    }

    /// Execute a command and stream its output as it arrives.
    pub async fn execute_streaming<F>(
        &self,
        cmd: &str,
        args: &[&str],
        mut on_output: F,
    ) -> CommandResult
    where
        F: FnMut(StreamSource, &str),
    {
        let mut child = match Command::new(cmd)
            .args(args)
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
        {
            Ok(child) => child,
            Err(error) => {
                return CommandResult {
                    success: false,
                    stdout: String::new(),
                    stderr: format!("Unable to start {cmd}: {error}"),
                };
            }
        };

        let (tx, mut rx) = mpsc::unbounded_channel();

        let stdout_task = child.stdout.take().map(|stdout| {
            let tx = tx.clone();
            tokio::spawn(Self::read_stream(stdout, tx, StreamSource::Stdout))
        });
        let stderr_task = child.stderr.take().map(|stderr| {
            let tx = tx.clone();
            tokio::spawn(Self::read_stream(stderr, tx, StreamSource::Stderr))
        });
        drop(tx);

        let collect_output = async {
            let mut stdout = Vec::new();
            let mut stderr = Vec::new();

            while let Some((source, line)) = rx.recv().await {
                on_output(source, &line);
                match source {
                    StreamSource::Stdout => stdout.push(line),
                    StreamSource::Stderr => stderr.push(line),
                }
            }

            (stdout.join("\n"), stderr.join("\n"))
        };

        let (status_result, (stdout, stderr)) = tokio::join!(child.wait(), collect_output);

        if let Some(task) = stdout_task {
            let _ = task.await;
        }
        if let Some(task) = stderr_task {
            let _ = task.await;
        }

        match status_result {
            Ok(status) => {
                let success = status.success();
                CommandResult {
                    success,
                    stdout,
                    stderr,
                }
            }
            Err(error) => CommandResult {
                success: false,
                stdout,
                stderr: if stderr.is_empty() {
                    error.to_string()
                } else {
                    format!("{stderr}\n{error}")
                },
            },
        }
    }

    /// Execute a command and collect the result without streaming output.
    pub async fn execute(&self, cmd: &str, args: &[&str]) -> CommandResult {
        self.execute_streaming(cmd, args, |_, _| {}).await
    }

    pub async fn sbofind(&self, query: &str) -> CommandResult {
        self.execute("sbofind", &[query]).await
    }

    /// Create a new user with useradd
    pub async fn useradd(&self, username: &str, groups: &[&str], shell: &str) -> CommandResult {
        let groups_str = groups.join(",");
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
                    },
                    Err(e) => CommandResult {
                        success: false,
                        stdout: String::new(),
                        stderr: e.to_string(),
                    },
                }
            }
            Err(e) => CommandResult {
                success: false,
                stdout: String::new(),
                stderr: e.to_string(),
            },
        }
    }
}

impl Default for CommandExecutor {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn missing_command_error_identifies_the_program() {
        let result = CommandExecutor::new()
            .execute("/nonexistent/slackware-cli-manager-test-command", &[])
            .await;
        assert!(!result.success);
        assert!(result.stdout.is_empty());
        assert!(result
            .stderr
            .contains("/nonexistent/slackware-cli-manager-test-command"));
    }
}
