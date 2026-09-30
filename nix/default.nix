let
  # Import nixpkgs if needed
  pkgs = import <nixpkgs> {};
in
  {
    lib ? pkgs.lib,
    buildGoModule ? pkgs.buildGoModule,
    installShellFiles ? pkgs.installShellFiles,
    # version and vendorHash should be specified by the caller
    version ? "latest",
    vendorHash,
  }:
    buildGoModule rec {
      pname = "packwiz";
      inherit version vendorHash;

      src = ./..;

      nativeBuildInputs = [
        installShellFiles
      ];

      # Install manual pages (there are no shell completions: the completion
      # command is disabled, see cmd/root.go)
      postInstall = ''
        $out/bin/packwiz man man
        installManPage man/*.1
      '';

      meta = with lib; {
        description = "A command line tool for editing and distributing Minecraft modpacks, using a git-friendly TOML format";
        homepage = "https://packwiz.infra.link/";
        license = licenses.mit;
        mainProgram = "packwiz";
      };
    }
