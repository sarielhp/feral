# Every target delegates to the canonical commands in tools/; the project
# contract they read is tools/settings.rb. Use tools/commit directly for
# messages with shell metacharacters.
.PHONY: all build gate check ci lint snapshot commit bump install release test test-live interactive interactive-live interactive-monkey interactive-check audit review static-analysis push clean

all: build

build:
	tools/build

gate:
	tools/gate

check:
	tools/check

ci:
	tools/gate --full

lint:
	tools/lint

snapshot:
	tools/snapshot $(if $(msg),"$(msg)")

commit:
	tools/commit "$(msg)"

bump:
	tools/bump

install:
	tools/install

# make release msg="..." [NO_INSTALL=1]; msg may be omitted on a clean tree.
release:
	tools/release $(if $(NO_INSTALL),--no-install) $(if $(msg),"$(msg)")

test:
	@mkdir -p /tmp/$(USER)/feral
	TMPDIR=/tmp/$(USER)/feral go test -timeout 180s ./...

test-live:
	ruby tools/test_live.rb

# Drives the TUI in kitty on a private Xvfb display; too slow for the gate.
# Fails, naming the packages, when a required tool is missing.
interactive:
	@mkdir -p /tmp/$(USER)/feral
	TMPDIR=/tmp/$(USER)/feral FERAL_INTERACTIVE=1 go test ./interactive/ -v -count=1

# The TUI in kitty against the seeded test account, read-only. Seed it first
# with tools/seed_test_account.rb. LIVE_ACCOUNT names the account.
LIVE_ACCOUNT ?= sariel_test
interactive-live:
	@mkdir -p /tmp/$(USER)/feral
	TMPDIR=/tmp/$(USER)/feral FERAL_INTERACTIVE=1 FERAL_LIVE=1 FERAL_LIVE_ACCOUNT=$(LIVE_ACCOUNT) \
		go test ./interactive/ -run Live -v -count=1

# Random keys at the live TUI, sandboxed and read-only. MONKEY_KEYS sets the length.
MONKEY_KEYS ?= 2000
# All three interactive suites in one run (about seven minutes); by hand, every so often.
interactive-check:
	tools/test_interactive

interactive-monkey:
	@mkdir -p /tmp/$(USER)/feral
	TMPDIR=/tmp/$(USER)/feral FERAL_INTERACTIVE=1 FERAL_LIVE=1 FERAL_LIVE_ACCOUNT=$(LIVE_ACCOUNT) \
		FERAL_LIVE_MONKEY=$(MONKEY_KEYS) go test ./interactive/ -run LiveMonkey -v -count=1 -timeout 60m

audit:
	tools/go-audit --strict --quiet .

review: static-analysis
static-analysis:
	tools/go-static-analysis

push:
	tools/push

clean:
	go clean
	rm -rf bin mail_cli feral
