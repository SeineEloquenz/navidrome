# Our fork built by reusing nixpkgs' navidrome derivation, swapping in our
# source and the hashes for our (post-upstream) go.mod and ui/package-lock.json.
# `plugins` is threaded through so the NixOS module's
# `package.override { plugins = …; }` keeps working.
{ navidrome, fetchNpmDeps, src, plugins ? [ ] }:

(navidrome.override { inherit plugins; }).overrideAttrs (final: prev: {
  pname = "navidrome-dl";
  version = "${prev.version}-dl";

  inherit src;

  vendorHash = "sha256-JopulJxWV7lNXkFu/5Nwfd6oS6qOV7wkCqAZcRnJXmk=";

  npmDeps = fetchNpmDeps {
    name = "navidrome-dl-npm-deps";
    src = src + "/ui";
    hash = "sha256-uRF9cf6HZE0gyCvGTEZ520d2gMsxmccEYLJBgc47pMg=";
  };
})
