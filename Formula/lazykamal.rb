# Homebrew formula for lazykamal.
# Install: brew install --build-from-source ./Formula/lazykamal.rb
# Or tap: brew tap lazykamal/lazykamal https://github.com/lazykamal/homebrew-lazykamal
#         brew install lazykamal

class Lazykamal < Formula
  desc "Lazydocker-style TUI for Kamal-deployed apps"
  homepage "https://github.com/lazykamal/lazykamal"
  url "https://github.com/lazykamal/lazykamal/archive/refs/tags/v1.0.0.tar.gz"
  sha256 ""  # Run: shasum -a 256 <(curl -sL https://github.com/lazykamal/lazykamal/archive/refs/tags/v1.0.0.tar.gz)
  license "MIT"
  head "https://github.com/lazykamal/lazykamal.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "."
  end

  test do
    assert_match "lazykamal", shell_output("#{bin}/lazykamal --version")
  end
end
