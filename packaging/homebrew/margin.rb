# Rendered by margin's release workflow from packaging/homebrew/margin.rb in
# github.com/neuroplastio/margin; change it there.
#
# A launcher plus a seed build (D20): the formula installs the thin
# launcher and a complete margin binary. On first run the wrapper seeds the
# launcher's home (~/.local/margin) from that binary, so margin works with no
# network and survives a channel outage; `margin update` then updates the copy
# in the home, outside the package manager.
class Margin < Formula
  desc "Terminal tool for reviewing markdown and handing the comments back to an agent"
  homepage "https://github.com/neuroplastio/margin"
  version "@VERSION@"
  license "Apache-2.0"

  if OS.mac? && Hardware::CPU.arm?
    url "https://github.com/neuroplastio/margin/releases/download/@VERSION@/margin-launcher_darwin_arm64"
    sha256 "@LAUNCHER_DARWIN_ARM64@"
    resource "seed" do
      url "https://github.com/neuroplastio/margin/releases/download/@VERSION@/margin_darwin_arm64"
      sha256 "@SEED_DARWIN_ARM64@"
    end
  elsif OS.mac?
    url "https://github.com/neuroplastio/margin/releases/download/@VERSION@/margin-launcher_darwin_amd64"
    sha256 "@LAUNCHER_DARWIN_AMD64@"
    resource "seed" do
      url "https://github.com/neuroplastio/margin/releases/download/@VERSION@/margin_darwin_amd64"
      sha256 "@SEED_DARWIN_AMD64@"
    end
  elsif Hardware::CPU.arm?
    url "https://github.com/neuroplastio/margin/releases/download/@VERSION@/margin-launcher_linux_arm64"
    sha256 "@LAUNCHER_LINUX_ARM64@"
    resource "seed" do
      url "https://github.com/neuroplastio/margin/releases/download/@VERSION@/margin_linux_arm64"
      sha256 "@SEED_LINUX_ARM64@"
    end
  else
    url "https://github.com/neuroplastio/margin/releases/download/@VERSION@/margin-launcher_linux_amd64"
    sha256 "@LAUNCHER_LINUX_AMD64@"
    resource "seed" do
      url "https://github.com/neuroplastio/margin/releases/download/@VERSION@/margin_linux_amd64"
      sha256 "@SEED_LINUX_AMD64@"
    end
  end

  def install
    libexec.install Dir["margin-launcher_*"].first => "margin-launcher"
    resource("seed").stage do
      (libexec/"margin-seed").install Dir["margin_*"].first => "margin"
    end
    # The release assets are bare binaries, which brew downloads without the
    # executable bit.
    chmod 0755, [libexec/"margin-launcher", libexec/"margin-seed/margin"]

    # bin/margin seeds the launcher's home from the packaged build on first
    # run, then hands over. The home is enlaunch's default (~/.local/margin);
    # an already-updated install is left alone.
    (bin/"margin").write <<~SH
      #!/bin/sh
      home="$HOME/.local/margin"
      if [ ! -e "$home/bin/margin" ]; then
        mkdir -p "$home/builds/@COMMIT@"
        cp "#{opt_libexec}/margin-seed/margin" "$home/builds/@COMMIT@/margin"
        chmod +x "$home/builds/@COMMIT@/margin"
        ln -sfn "builds/@COMMIT@" "$home/bin"
      fi
      exec "#{opt_libexec}/margin-launcher" "$@"
    SH
    chmod 0755, bin/"margin"
  end

  test do
    ENV["HOME"] = testpath.to_s
    assert_match "margin", shell_output("#{bin}/margin --version")
  end
end
