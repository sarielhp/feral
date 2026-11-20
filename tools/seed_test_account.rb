# frozen_string_literal: true

# seed_test_account.rb -- fill a throwaway test account with a realistic mailbox.
#
# The mail comes from the public Enron corpus (CMU, enron_mail_20150507): a few
# employees' real folder trees, capped to about 1500 messages and laid out four
# levels deep. Every e-mail address -- in headers and in bodies -- is replaced
# by a pseudonym derived from a keyed hash of the address, so the same person
# is the same pseudonym everywhere and no message can reach anyone: every
# pseudonym is on a reserved .example domain (RFC 2606). Display names are left
# as the corpus has them.
#
# A handful of synthetic canary messages with known subjects go into fixed
# folders, so a live test can assert on exact content.
#
# Usage:
#   tools/seed_test_account.rb --corpus DIR [--out DIR] --build
#   tools/seed_test_account.rb --corpus DIR --account NAME --upload
#
# --build writes the mbox files and manifest.json and touches nothing else.
# --upload builds, then uploads through feral, and only to an account whose
# account_type is "test"; it refuses when a folder it would create exists.

require 'digest'
require 'fileutils'
require 'json'
require 'open3'
require 'openssl'
require 'optparse'

HASH_KEY = 'mail_cli-enron-seed-v1'

# The employees whose folders are used, and where their trees go. The owner's
# inbox, sent and deleted mail fill the account's own well-known folders; the
# others' folders hang under Colleagues/<name>, which is what makes the tree
# four levels deep.
OWNER = 'kaminski-v'
COLLEAGUES = %w[germany-c kean-s beck-s].freeze
# How many of each colleague's top-level folders are taken, nested ones first.
COLLEAGUE_FOLDERS = 8

WELL_KNOWN = {
  'inbox' => 'Inbox',
  'sent' => 'Sent Items',
  'deleted_items' => 'Deleted Items'
}.freeze

# Folders that only duplicate others (Lotus Notes views, the second sent
# folder, the all-documents view) and would fill the account with copies.
SKIPPED = %w[
  all_documents discussion_threads notes_inbox sent_items _sent_mail
  _sent contacts calendar
].freeze

# Messages taken per folder, newest first.
CAPS = { 'Inbox' => 200, 'Sent Items' => 120, 'Deleted Items' => 60, folder: 12, colleague: 10 }.freeze
MESSAGE_BUDGET = 2000
MAX_DEPTH = 4

# Exchange keeps hidden non-mail folders by these names at the top of every
# mailbox, and refuses a top-level mail folder with one of them
# (ErrorFolderExists); nested, they are ordinary names.
RESERVED_NAMES = %w[tasks calendar contacts notes journal].freeze

FIRST_NAMES = %w[
  avery blake casey dana eden finley gray harper indigo jules kai logan
  morgan noel oakley parker quinn reese sage tatum umber vale wren xen
  yael zion arden briar cyan dell ember fable
].map(&:freeze).freeze

LAST_NAMES = %w[
  abbot birch cole dunmore ellery frost garnet hollis ingram juniper keene
  larkin mercer nash orchard pryor quill rowan sutter thorne underhill
  vance whitlock yarrow zeller ashby brook crane delacroix everly fenwick
].map(&:freeze).freeze

ADDRESS_RE = /[A-Za-z0-9._%+'-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+/n
FROM_LINE_RE = /^From /n
# Headers whose values are identifiers rather than addresses; rewriting them
# would break threading for no gain, since they deliver nowhere.
ID_HEADER_RE = /\A(?:message-id|in-reply-to|references):/i

# Pseudonym is the keyed-hash mapping of real addresses to reserved ones.
module Seed
  class Pseudonym
    def initialize
      @cache = {}
    end

    def address(real)
      key = real.downcase
      @cache[key] ||= build(key)
    end

    def rewrite(text)
      text.gsub(ADDRESS_RE) { |found| address(found) }
    end

    def size
      @cache.size
    end

    private

    def build(key)
      domain = key.split('@', 2).last
      digest = OpenSSL::HMAC.digest('SHA256', HASH_KEY, key).bytes
      first = FIRST_NAMES[digest[0] % FIRST_NAMES.size]
      last = LAST_NAMES[digest[1] % LAST_NAMES.size]
      tag = digest[2, 2].map { |b| format('%02x', b) }.join
      "#{first}.#{last}.#{tag}@#{pseudo_domain(domain)}"
    end

    def pseudo_domain(domain)
      return 'enron.example' if domain == 'enron.com' || domain.end_with?('.enron.com')

      digest = OpenSSL::HMAC.hexdigest('SHA256', HASH_KEY, domain)
      "corp-#{digest[0, 6]}.example"
    end
  end

  class Plan
    attr_reader :folders

    def initialize(corpus)
      @corpus = corpus
      @folders = {}
    end

    def total
      @folders.values.sum(&:size)
    end

    def add(server_path, files, cap)
      room = MESSAGE_BUDGET - total
      return if room <= 0

      # An empty folder is kept: real accounts have them, and a deeper folder needs its parent.
      @folders[server_path] = files.last([cap, room].min)
    end

    def add_owner
      root = File.join(@corpus, OWNER)
      subdirs(root).each do |dir|
        name = File.basename(dir)
        next if SKIPPED.include?(name)

        target = WELL_KNOWN.fetch(name) { top_level_name(display_name(name)) }
        cap = CAPS.fetch(target, CAPS[:folder])
        add_tree(dir, target, cap)
      end
    end

    def add_colleagues
      COLLEAGUES.each do |user|
        root = File.join(@corpus, user)
        next unless File.directory?(root)

        colleague_dirs(root).each do |dir|
          add_tree(dir, "Colleagues/#{user}/#{display_name(File.basename(dir))}", CAPS[:colleague])
        end
      end
    end

    private

    # colleague_dirs picks a colleague's folders, those with subfolders first.
    def colleague_dirs(root)
      dirs = subdirs(root).reject { |d| SKIPPED.include?(File.basename(d)) || WELL_KNOWN.key?(File.basename(d)) }
      dirs.sort_by { |d| [subdirs(d).empty? ? 1 : 0, d] }.first(COLLEAGUE_FOLDERS)
    end

    # add_tree adds dir and its subfolders, stopping at MAX_DEPTH levels.
    def add_tree(dir, target, cap)
      add(target, message_files(dir), cap)
      return if target.count('/') + 1 >= MAX_DEPTH

      subdirs(dir).each { |sub| add_tree(sub, "#{target}/#{display_name(File.basename(sub))}", cap) }
    end

    def top_level_name(shown)
      RESERVED_NAMES.include?(shown.downcase) ? "#{shown} Mail" : shown
    end

    def display_name(name)
      name.tr('_', ' ').strip.split.map(&:capitalize).join(' ')
    end
  end
end

# A message ready for the mbox: the rewritten bytes and its subject.
Message = Struct.new(:raw, :subject, :from, keyword_init: true)

def anonymize(raw, pseudonym)
  head, sep, body = raw.partition(/\r?\n\r?\n/n)
  lines = head.split(/(?<=\n)/n).map do |line|
    ID_HEADER_RE.match?(line) ? line : pseudonym.rewrite(line)
  end
  lines.join + sep + pseudonym.rewrite(body)
end

def header_value(raw, name)
  head = raw.partition(/\r?\n\r?\n/n).first
  match = head.match(/^#{name}:[ \t]*(.*)$/in)
  match ? match[1].strip.force_encoding('UTF-8').scrub : ''
end

def load_message(path, pseudonym)
  raw = anonymize(File.binread(path), pseudonym)
  Message.new(raw: raw, subject: header_value(raw, 'Subject'), from: header_value(raw, 'From'))
end

# Corpus file names are "1.", "2.", ...; numeric order is the order received.
def message_files(dir)
  Dir.children(dir).map { |n| File.join(dir, n) }.select { |p| File.file?(p) }
     .sort_by { |p| File.basename(p).to_i }
end

def subdirs(dir)
  Dir.children(dir).map { |n| File.join(dir, n) }.select { |p| File.directory?(p) }.sort
end

# Plan maps a server folder path to the corpus files it receives.

CANARY_FROM = 'Canary Sender <canary.sender@mail-cli.example>'

# Canaries are synthetic messages a live test asserts on by exact subject.
CANARIES = [
  { folder: 'Inbox', subject: 'mail_cli canary: inbox plain',
    body: "The word zanzibarquux appears only in this message.\n" },
  { folder: 'Inbox', subject: 'mail_cli canary: שלום עולם mixed direction',
    body: "Hebrew subject for the bidi paths.\n" },
  { folder: 'Colleagues/kean-s/Canary/Level3', subject: 'mail_cli canary: deep folder',
    body: "This message sits four levels down.\n" }
].map(&:freeze).freeze

def canary_raw(canary, index)
  date = Time.utc(2001, 12, 1, 9, index).strftime('%a, %d %b %Y %H:%M:%S +0000')
  subject = canary[:subject]
  subject = "=?utf-8?B?#{[subject].pack('m0')}?=" unless subject.ascii_only?
  ["Message-ID: <canary-#{index}@mail-cli.example>", "Date: #{date}", "From: #{CANARY_FROM}",
   'To: Test Owner <owner@mail-cli.example>', "Subject: #{subject}", 'MIME-Version: 1.0',
   'Content-Type: text/plain; charset=utf-8', '', canary[:body]].join("\n").b
end

def mbox_entry(raw)
  body = raw.gsub(FROM_LINE_RE, '>From ')
  body << "\n" unless body.end_with?("\n")
  "From MAILER-DAEMON Thu Jan  1 00:00:00 1970\n".b + body + "\n"
end

def mbox_name(folder)
  folder.gsub(/[^A-Za-z0-9]+/, '_') + '.mbox'
end

# build writes one mbox per folder and the manifest, returning the manifest.
def build(corpus, out)
  plan = Seed::Plan.new(corpus)
  plan.add_owner
  plan.add_colleagues
  pseudonym = Seed::Pseudonym.new
  FileUtils.mkdir_p(out)
  manifest = { 'source' => 'enron_mail_20150507', 'folders' => {}, 'canaries' => [] }

  plan.folders.each do |folder, files|
    messages = files.map { |f| load_message(f, pseudonym) }
    File.binwrite(File.join(out, mbox_name(folder)), messages.map { |m| mbox_entry(m.raw) }.join)
    manifest['folders'][folder] = {
      'mbox' => mbox_name(folder), 'count' => messages.size,
      'subjects' => messages.map(&:subject).reject(&:empty?).first(5)
    }
  end
  add_canaries(out, manifest)
  manifest['addresses_rewritten'] = pseudonym.size
  File.write(File.join(out, 'manifest.json'), JSON.pretty_generate(manifest))
  manifest
end

def add_canaries(out, manifest)
  CANARIES.group_by { |c| c[:folder] }.each do |folder, canaries|
    name = mbox_name(folder).sub('.mbox', '_canary.mbox')
    entries = canaries.map { |c| mbox_entry(canary_raw(c, CANARIES.index(c))) }
    File.binwrite(File.join(out, name), entries.join)
    canaries.each { |c| manifest['canaries'] << { 'folder' => folder, 'subject' => c[:subject], 'mbox' => name } }
  end
end

def feral(account, *)
  out, status = Open3.capture2e('feral', '--account', account, *)
  [out, status.success?]
end

def require_test_account!(account)
  out, ok = feral(account, '--json', 'account', 'list')
  abort "feral account list failed:\n#{out}" unless ok
  entry = JSON.parse(out).find { |a| a['name'].casecmp?(account) }
  abort "account #{account.inspect} is not configured" unless entry
  return if entry['account_type'] == 'test'

  abort "refusing to seed #{account.inspect}: account_type is #{entry['account_type'].inspect}, not \"test\""
end

def existing_folders(account)
  out, ok = feral(account, 'label', 'print')
  abort "listing folders failed:\n#{out}" unless ok
  out.lines.to_set { |l| l.strip.downcase }
end

def refuse_reseed!(account, manifest)
  have = existing_folders(account)
  created = manifest['folders'].keys.reject { |f| WELL_KNOWN.value?(f) }
  clash = created.find { |f| have.include?(f.downcase) }
  abort "#{account} already has #{clash.inspect}; it looks seeded. Remove the seeded folders first." if clash
end

def upload_one(account, folder, path)
  out, ok = feral(account, 'label', 'create', folder)
  warn "create #{folder}: #{out.strip}" unless ok || out.include?('exist')
  # upload takes a label reference, which carries the % sigil; create takes a name.
  out, ok = feral(account, 'label', 'upload', "%#{folder}", path)
  warn "upload #{folder} failed:\n#{out}" unless ok
  puts "#{ok ? 'ok  ' : 'FAIL'} #{folder}"
  ok
end

def upload(account, out, manifest)
  require_test_account!(account)
  refuse_reseed!(account, manifest)
  jobs = manifest['folders'].map { |folder, info| [folder, info['mbox']] } +
         manifest['canaries'].map { |c| [c['folder'], c['mbox']] }.uniq
  failed = jobs.reject { |folder, mbox| upload_one(account, folder, File.join(out, mbox)) }
  abort "#{failed.size} folder(s) failed: #{failed.map(&:first).join(', ')}" unless failed.empty?
end

def parse_options
  opts = { out: File.expand_path('~/.cache/mail_cli_testing/enron/seed') }
  OptionParser.new do |o|
    o.banner = 'Usage: seed_test_account.rb --corpus DIR (--build | --account NAME --upload)'
    o.on('--corpus DIR', 'the maildir/ directory of the Enron corpus') { |v| opts[:corpus] = v }
    o.on('--out DIR', 'where the mbox files and manifest go') { |v| opts[:out] = v }
    o.on('--account NAME', 'the test account to fill') { |v| opts[:account] = v }
    o.on('--build', 'write mbox files and manifest only') { opts[:build] = true }
    o.on('--upload', 'build, then upload to --account') { opts[:upload] = true }
  end.parse!
  opts
end

opts = parse_options
abort '--corpus is required' unless opts[:corpus] && File.directory?(opts[:corpus])
abort 'give --build or --upload' unless opts[:build] || opts[:upload]
abort '--upload needs --account' if opts[:upload] && !opts[:account]

manifest = build(opts[:corpus], opts[:out])
total = manifest['folders'].values.sum { |f| f['count'] }
puts "built #{manifest['folders'].size} folders, #{total} messages, " \
     "#{manifest['addresses_rewritten']} addresses rewritten -> #{opts[:out]}"
upload(opts[:account], opts[:out], manifest) if opts[:upload]
