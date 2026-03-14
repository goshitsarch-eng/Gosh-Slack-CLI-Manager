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
use utils::root::is_root;

const APP_NAME: &str = "Slackware CLI Manager";
const VERSION: &str = env!("CARGO_PKG_VERSION");

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let running_as_root = is_root();

    let version = match detect_version() {
        Ok(v) => {
            println!("{} v{}", APP_NAME, VERSION);
            println!("Detected: {}", v.display_name());
            if !running_as_root {
                println!("Running without root privileges. Mutating actions will be disabled.");
            }
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

    install_panic_hook();
    enable_raw_mode()?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen, EnableMouseCapture)?;
    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend)?;

    let mut app = App::new(version, running_as_root);

    let result = run_app(&mut terminal, &mut app).await;

    restore_terminal(&mut terminal)?;

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
        terminal.draw(|frame| app.render(frame))?;

        if event::poll(Duration::from_millis(100))? {
            if let Event::Key(key) = event::read()? {
                if let Some(msg) = app.handle_input(key) {
                    app.update(msg);
                }
            }
        }

        while let Ok(msg) = app.event_rx.try_recv() {
            app.update(msg);
        }

        if !app.running {
            break;
        }
    }

    Ok(())
}

fn install_panic_hook() {
    std::panic::set_hook(Box::new(|panic_info| {
        let _ = disable_raw_mode();
        let mut stdout = io::stdout();
        let _ = execute!(stdout, LeaveAlternateScreen, DisableMouseCapture);
        eprintln!("{panic_info}");
    }));
}

fn restore_terminal(terminal: &mut Terminal<CrosstermBackend<io::Stdout>>) -> anyhow::Result<()> {
    disable_raw_mode()?;
    execute!(
        terminal.backend_mut(),
        LeaveAlternateScreen,
        DisableMouseCapture
    )?;
    terminal.show_cursor()?;
    Ok(())
}
