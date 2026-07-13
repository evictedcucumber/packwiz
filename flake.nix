{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/master";
    flake-utils.url = "github:numtide/flake-utils";

    rust-overlay.url = "github:oxalica/rust-overlay";
    rust-overlay.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = {
    nixpkgs,
    rust-overlay,
    flake-utils,
    ...
  }:
    flake-utils.lib.eachSystem flake-utils.lib.allSystems (system: let
      pkgs = import nixpkgs {
        inherit system;

        config.allowUnfree = true;

        overlays = [(import rust-overlay)];
      };
    in {
      devShells.default = pkgs.mkShell {
        name = "packwiz";
        packages = with pkgs; [
          (rust-bin.fromRustupToolchainFile ./rust-toolchain.toml)
          cargo-audit
          cargo-tarpaulin
          cargo-nextest
          cargo-deny
          bacon
        ];
      };
    });
}
