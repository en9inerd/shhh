class ShhhCli < Formula
  desc "CLI for shhh E2E encrypted channels"
  homepage "https://github.com/en9inerd/shhh"
  license "MIT"
  version "VERSION_PLACEHOLDER"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/en9inerd/shhh/releases/download/vVERSION_PLACEHOLDER/shhh-cli-darwin-arm64"
      sha256 "SHA256_MACOS_ARM64"
    else
      url "https://github.com/en9inerd/shhh/releases/download/vVERSION_PLACEHOLDER/shhh-cli-darwin-amd64"
      sha256 "SHA256_MACOS_AMD64"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/en9inerd/shhh/releases/download/vVERSION_PLACEHOLDER/shhh-cli-linux-arm64"
      sha256 "SHA256_LINUX_ARM64"
    else
      url "https://github.com/en9inerd/shhh/releases/download/vVERSION_PLACEHOLDER/shhh-cli-linux-amd64"
      sha256 "SHA256_LINUX_AMD64"
    end
  end

  def install
    binary = Dir["shhh-cli*"].first
    bin.install binary => "shhh-cli"
  end

  test do
    assert_match "shhh-cli version", shell_output("#{bin}/shhh-cli --version")
  end
end
