use clap::Parser;
use packwiz::cli::Cli;

fn main() {
    let cli = Cli::parse();

    dbg!(cli);
}
