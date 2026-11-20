# frozen_string_literal: true

# Project choices for the shared tooling in tools/tooling.rb. Copied from
# ~/prog/standards/go/templates/settings.rb and adapted; the contract is in that
# directory's README.
module ToolSettings
  TOOLS = {}.freeze # Optional exact executable paths, e.g. { 'go-audit' => '/path/go-audit' }
  BUILD_PACKAGES = ['./...'].freeze
  # Declared rather than detected: detection names the binary after the checkout
  # directory, so a clone or worktree called anything else would build
  # bin/<directory> and then reject bin/feral as "not a declared artifact".
  ARTIFACTS = { 'bin/feral' => '.' }.freeze
  INSTALL_TARGETS = { 'bin/feral' => File.expand_path('~/bin/feral') }.freeze
  # Names installed as symlinks to an installed target, link => target name.
  # The program was mail_cli; scripts, wrappers, cron jobs and alias links
  # still run that name, and a binary left under it would be a stale build
  # that no later install replaces.
  INSTALL_LINKS = { File.expand_path('~/bin/mail_cli') => 'feral' }.freeze
  VERSION_FILE = 'VERSION'
  # Version is a compiled-in constant (app/constants.go), so a bump re-verifies
  # the whole tree rather than reusing a prior fast gate.
  VERSION_KIND = :embedded
  # VERSION is authoritative: sync_version.rb writes it into app/constants.go,
  # then the generated help pages (whose index prints the version) are rebuilt.
  VERSION_UPDATE = [['ruby', 'tools/sync_version.rb'], ['go', 'run', '.', 'docs']].freeze
  # arch_check.rb is the layering guard; sync_version.rb --check keeps VERSION
  # and app/constants.go from drifting apart.
  FAST_CHECKS = [['ruby', 'tools/arch_check.rb'],
                 ['ruby', 'tools/sync_version.rb', '--check']].freeze
  FULL_CHECKS = [].freeze
  TEST_TIMEOUT = '180s'
  VERIFICATION_INPUTS = [].freeze # Explicit extra paths/globs for ignored inputs read by project checks.

  # tools/publish copies a release to the public repository as one commit per
  # version; development happens in the private sarielhp/feral_dev (formerly
  # mail_cli_dev). These
  # paths are development records and agent configuration, and stay private.
  # Symbolic links are left out as well: the ones here point into
  # ~/prog/standards, which a public checkout does not have.
  PUBLIC_REMOTE = 'git@github.com:sarielhp/feral.git'
  PUBLIC_BRANCH = 'main'
  PUBLISH_EXCLUDE = %w[
    AGENTS.md opencode.json .bwrc .bws .agents
    reviews todo plans prompts docs/instructions
    tools/REVIEW_CYCLE.md tools/review_cycle.json tools/mcp_github_setup
    tools/publish tools/publish-allowed-addresses.txt
  ].freeze
  # A bump moves the go.mod toolchain line to the latest patch of the Go in
  # use (go get toolchain@patch) before the full gate, so a release builds with
  # a patched standard library. false leaves go.mod alone.
  UPDATE_TOOLCHAIN = true
  # Every address in a published file must be at a domain reserved for
  # examples or listed in this file, so a real one cannot slip out unnoticed.
  PUBLISH_ALLOWED_ADDRESSES = 'tools/publish-allowed-addresses.txt'
end
