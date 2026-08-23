{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  # This maps to https://github.com/NixOS/nixpkgs/tree/nixos-unstable
  # The `url` option is the pattern of `github:USER_OR_ORG/REPO/BRANCH`

  outputs = {
    self,
    nixpkgs,
  }:
    with nixpkgs.lib; let
      # List of explicitly unsupported systems
      explicitlyUnsupportedSystems = [];

      # Packwiz should support all 64-bit systems supported by go, but nix only
      # support strictly less, so all nix-supported systems are included
      # (except ones in explicitlyUnsupportedSystems).
      supportedSystems =
        filter
        # Filter out systems that are explicitly unsupported
        (s: ! elem s explicitlyUnsupportedSystems)
        # This lists all systems reasonably well-supported by nix
        nixpkgs.lib.systems.flakeExposed;

      # Helper generating outputs for each supported system
      forAllSystems = genAttrs supportedSystems;

      # Import nixpkgs' package set for each system.
      nixpkgsFor = forAllSystems (system: import nixpkgs {inherit system;});
    in {
      # Packwiz package
      packages = forAllSystems (system: let
        pkgs = nixpkgsFor.${system};
      in rec {
        packwiz = pkgs.callPackage ./nix {
          version = substring 0 8 self.rev or "dirty";
          # Bump this whenever go.mod/go.sum change: clear the file, run
          # `nix build`, and copy the "got:" hash from the failed build
          # back into nix/vendor-hash.
          vendorHash = fileContents ./nix/vendor-hash;
        };
        # Build packwiz by default when no package name is specified
        default = packwiz;
      });

      # Development shell with the toolchain needed to build packwiz
      devShells = forAllSystems (system: let
        pkgs = nixpkgsFor.${system};
      in {
        default = pkgs.mkShell {
          inputsFrom = [self.packages.${system}.packwiz];
          packages = [pkgs.gopls];
        };
      });

      # This flake's nix code formatter
      formatter = forAllSystems (system: nixpkgsFor.${system}.alejandra);
    };
}
