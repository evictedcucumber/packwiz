use clap::{Parser, Subcommand};

#[derive(Parser, Debug)]
#[command(version,about, long_about = None)]
pub struct Cli {
    #[command(subcommand)]
    pub commands: Commands,
}

#[derive(Subcommand, Debug)]
pub enum Commands {
    Add {
        #[arg(value_parser = validation::modrinth_url)]
        url: Item,
    },
}

#[derive(Clone, Debug)]
pub struct Item {
    pub category: String,
    pub name: String,
}

mod validation {
    use super::Item;
    use regex::Regex;
    use std::sync::LazyLock;

    static MODRINTH_URL_REGEX: LazyLock<Regex> = LazyLock::new(|| {
        Regex::new(r"^https://modrinth\.com/(mod|resourcepack|shaderpack)/(.+)/?$").unwrap()
    });

    pub fn modrinth_url(url: &str) -> Result<Item, String> {
        let captures = MODRINTH_URL_REGEX
            .captures(url)
            .ok_or("Not a valid modrinth URL")?;

        Ok(Item {
            category: captures[1].to_string(),
            name: captures[2].to_string(),
        })
    }

    #[cfg(test)]
    mod tests {
        use super::*;

        #[test]
        fn should_validate() {
            assert!(modrinth_url("https://modrinth.com/mod/mod-name").is_ok());
            assert!(modrinth_url("https://modrinth.com/resourcepack/resourcepack-name").is_ok());
            assert!(modrinth_url("https://modrinth.com/shaderpack/shaderpack-name").is_ok());

            assert!(modrinth_url("https://modrinth.com/mod/mod-name/").is_ok());
            assert!(modrinth_url("https://modrinth.com/resourcepack/resourcepack-name/").is_ok());
            assert!(modrinth_url("https://modrinth.com/shaderpack/shaderpack-name/").is_ok());
        }

        #[test]
        fn should_invalidate_missing_slug() {
            assert!(modrinth_url("https://modrinth.com/mod/").is_err());
            assert!(modrinth_url("https://modrinth.com/resourcepack/").is_err());
            assert!(modrinth_url("https://modrinth.com/shaderpack/").is_err());
        }

        #[test]
        fn should_invalidate_wrong_category() {
            assert!(modrinth_url("https://modrinth.com/unknown/unknown-name").is_err());
            assert!(modrinth_url("https://modrinth.com/unknown/unknown-name/").is_err());
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn cli_debug_assert() {
        use clap::CommandFactory;

        Cli::command().debug_assert();
    }
}
