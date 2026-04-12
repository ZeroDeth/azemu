{
  description = "azemu - local Azure emulator for Terraform";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "azemu";
          version = "0.1.0-dev";
          src = ./.;
          # To update: run `nix build` and replace with the hash from the error.
          vendorHash = null;
          subPackages = [ "cmd/azemu" ];
          ldflags = [ "-X" "main.Version=0.1.0-dev" ];
          meta = {
            description = "Local Azure emulator for Terraform-first development";
            homepage = "https://github.com/zerodeth/azemu";
            license = pkgs.lib.licenses.mit;
            mainProgram = "azemu";
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_22
            terraform
            pre-commit
            jq
            shellcheck
          ];
        };
      }
    );
}
