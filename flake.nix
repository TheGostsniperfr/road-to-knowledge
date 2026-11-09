{
  description = "Road to Knowledge: a zoomable universe of what you know";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";

  outputs = { nixpkgs, ... }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAll = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in {
      # Development environment only: the server runs with `make serve`, no NixOS module on purpose.
      devShells = forAll (pkgs: {
        default = pkgs.mkShell {
          # poppler-utils: pdftotext/pdfinfo for book-ingest, and pdftoppm, which Claude Code needs to read PDF page ranges.
          packages = with pkgs; [ go gopls gnumake nodejs_24 git gh jq poppler-utils ];
        };
      });
    };
}
