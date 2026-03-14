use std::fs::{self, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use super::error::{AppError, Result};

fn temp_path_for(path: &Path) -> Result<PathBuf> {
    let parent = path.parent().ok_or_else(|| {
        AppError::FileOperation(format!("{} has no parent directory", path.display()))
    })?;
    let file_name = path
        .file_name()
        .and_then(|name| name.to_str())
        .ok_or_else(|| {
            AppError::FileOperation(format!("{} has no valid file name", path.display()))
        })?;
    let nonce = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_err(|err| AppError::FileOperation(format!("Clock error: {err}")))?
        .as_nanos();

    Ok(parent.join(format!(".{file_name}.tmp.{nonce}")))
}

pub fn atomic_write(path: impl AsRef<Path>, contents: &str) -> Result<()> {
    let path = path.as_ref();
    let temp_path = temp_path_for(path)?;

    let mut temp_file = OpenOptions::new()
        .create_new(true)
        .write(true)
        .open(&temp_path)?;

    if let Ok(metadata) = fs::metadata(path) {
        temp_file.set_permissions(metadata.permissions())?;
    }

    temp_file.write_all(contents.as_bytes())?;
    temp_file.sync_all()?;
    drop(temp_file);

    fs::rename(&temp_path, path)?;

    if let Some(parent) = path.parent() {
        if let Ok(dir) = OpenOptions::new().read(true).open(parent) {
            let _ = dir.sync_all();
        }
    }

    Ok(())
}
