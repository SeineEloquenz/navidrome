{
  description = "Navidrome fork with SomeDL download integration";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      lib = nixpkgs.lib;
      fs = lib.fileset;

      # Scope the source so changes to our tooling (nix/, flake, CI) don't force
      # a rebuild of the server.
      navidromeSrc = {
        outPath = fs.toSource {
          root = ./.;
          fileset = fs.difference ./. (fs.unions [
            (fs.maybeMissing ./nix)
            (fs.maybeMissing ./flake.nix)
            (fs.maybeMissing ./flake.lock)
            (fs.maybeMissing ./.github)
            (fs.maybeMissing ./deps)
          ]);
        };
        # nixpkgs' navidrome ldflags read src.rev; a clean flake self has it.
        rev = self.rev or "dl";
      };

      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = lib.genAttrs systems;
      packagesFor = pkgs: {
        navidrome-dl = pkgs.callPackage ./nix/navidrome.nix { src = navidromeSrc; };
      };
    in
    {
      overlays.default = final: _prev: packagesFor final;

      packages = forAllSystems (system:
        let ps = packagesFor nixpkgs.legacyPackages.${system};
        in ps // { default = ps.navidrome-dl; });
    };
}
