{
  description = "Development environment for Amber";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";

  outputs = {nixpkgs, ...}: let
    systems = [
      "aarch64-darwin"
      "aarch64-linux"
      "x86_64-linux"
    ];
    forAllSystems = nixpkgs.lib.genAttrs systems;
  in {
    devShells = forAllSystems (system: let
      pkgs = nixpkgs.legacyPackages.${system};
    in {
      default = pkgs.mkShell {
        # Avoid inheriting GOROOT from another Go installation (e.g. mise).
        GOROOT = "${pkgs.go}/share/go";

        packages = with pkgs; [
          go
          gofumpt
          gopls
          gnumake
          git
        ];
      };
    });
  };
}
