mod app;
mod components;
mod slackware;
mod ui;
mod utils;

use std::io;
use std::time::Duration;

use crossterm::{
    event::{self, DisableMouseCapture, EnableMouseCapture, Event},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use ratatui::{backend::CrosstermBackend, Terminal};

use app::App;
use slackware::detect_version;
use utils::check_root;

const APP_NAME: &str = "Slackware CLI Manager";
const VERSION: &str = env!("CARGO_PKG_VERSION");

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Check for root privileges
    if let Err(e) = check_root() {
        eprintln!("{}", e);
        eprintln!("\nThis application requires root privileges to manage Slackware packages.");
        eprintln!("Please run with: sudo {}", std::env::args().next().unwrap_or_default());
        std::process::exit(1);
    }

    // Detect Slackware version
    let version = match detect_version() {
        Ok(v) => {
            println!("{} v{}", APP_NAME, VERSION);
            println!("Detected: {}", v.display_name());
            v
        }
        Err(e) => {
            eprintln!("Warning: {}", e);
            eprintln!("Proceeding with default settings (Slackware Current)");
            slackware::SlackwareVersion::Current
        }
    };

    // Brief pause to show version info
    std::thread::sleep(Duration::from_millis(500));

    // Setup terminal
    enable_raw_mode()?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen, EnableMouseCapture)?;
    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend)?;

    // Create app
    let mut app = App::new(version);

    // Run the app
    let result = run_app(&mut terminal, &mut app).await;

    // Restore terminal
    disable_raw_mode()?;
    execute!(
        terminal.backend_mut(),
        LeaveAlternateScreen,
        DisableMouseCapture
    )?;
    terminal.show_cursor()?;

    if let Err(e) = result {
        eprintln!("Error: {}", e);
        std::process::exit(1);
    }

    println!("\nThank you for using {}!", APP_NAME);
    Ok(())
}

async fn run_app(
    terminal: &mut Terminal<CrosstermBackend<io::Stdout>>,
    app: &mut App,
) -> anyhow::Result<()> {
    loop {
        // Draw UI
        terminal.draw(|frame| app.render(frame))?;

        // Handle events
        if event::poll(Duration::from_millis(100))? {
            if let Event::Key(key) = event::read()? {
                if let Some(msg) = app.handle_input(key) {
                    app.update(msg).await;
                }
            }
        }

        // Check for progress updates
        while let Ok(line) = app.progress_rx.try_recv() {
            app.update(app::Message::ProgressUpdate(line)).await;
        }

        // Exit if not running
        if !app.running {
            break;
        }
    }

    Ok(())
}
