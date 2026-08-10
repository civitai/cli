# FIRE-DRILL FIXTURE — not the tap, not shipped to anyone.
#
# The LAG shape, which is the opposite of the outage in the sibling fixture:
# every archive URL below is real and publicly downloadable, so no user is
# broken — the cask simply names an older release than the newest published one.
# Only the lag finding can fire on this file, which is what makes it a control
# on the two findings being distinguishable rather than one detector with two
# labels.
#
# v0.1.90 is deliberately a REAL published release. If it is ever deleted this
# fixture starts reporting `broken` instead of `lagging`; that is a broken
# fixture, not a finding — repoint it at any current release.
#
# Reached only by `gh workflow run release-homebrew.yml -f drill=lagging`, which
# points tools/caskcheck at this file with `-lag-threshold 1s`, so the newest
# published release counts as "published long ago" whenever the drill is run.
cask "civitai" do
  version "0.1.90"

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
