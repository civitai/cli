# FIRE-DRILL FIXTURE — not the tap, not shipped to anyone.
#
# This is what the 2026-08-09 outage looked like from `brew install`'s side: a
# cask naming a version whose release archives are not publicly downloadable.
# The version below has never been released and never will be, so these are real
# github.com URLs that answer a real 404 to a real unauthenticated request —
# the drill exercises the same network path the live check does, not a stub.
#
# Reached only by `gh workflow run release-homebrew.yml -f drill=broken`, which
# points tools/caskcheck at this file over raw.githubusercontent. See the
# `drill` input in .github/workflows/release-homebrew.yml.
cask "civitai" do
  version "0.0.0-firedrill"

  on_macos do
    on_intel do
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
      url "https://github.com/civitai/cli/releases/download/v#{version}/civitai_#{version}_darwin_amd64.tar.gz"
    end
    on_arm do
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
      url "https://github.com/civitai/cli/releases/download/v#{version}/civitai_#{version}_darwin_arm64.tar.gz"
    end
  end

  binary "civitai"
end
