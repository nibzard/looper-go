class Looper < Formula
  desc "Deterministic autonomous loop runner for AI agents"
  homepage "https://github.com/nibzard/looper"
  url "https://github.com/nibzard/looper/archive/refs/tags/v0.3.3.tar.gz"
  sha256 "47dfb9dbcb842eefb9520c9f548e764ec77f88081736b1bab1c462730e960d17"

  head "https://github.com/nibzard/looper.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-X main.Version=#{version}"), "./cmd/looper"

    # Install optional Codex skills (not installed by default)
    # Users can run: looper-install --with-skills
    bin.install "install.sh" => "looper-install"
    bin.install "uninstall.sh" => "looper-uninstall"
    pkgshare.install "skills"
    pkgshare.install "README.md"
  end

  def caveats
    <<~EOS
      The looper binary has been installed.

      To optionally install Codex skills into ~/.codex/skills:
        looper-install --with-skills --skip-bin
    EOS
  end

  test do
    system "#{bin}/looper", "version"
    system "#{bin}/looper", "--help"
  end
end
