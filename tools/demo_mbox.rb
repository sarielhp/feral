# frozen_string_literal: true

# demo_mbox.rb -- turn testdata/demo/mailbox.yml, the synthetic mailbox the
# README's demo GIF is recorded on, into one mbox file per account.
#
# Every person, address and list in it is made up and on a domain reserved for
# examples, so nothing can reach anyone and tools/publish accepts it. The
# output is the same bytes on every run: dates are fixed, nothing is random.
#
# Usage:
#   ruby tools/demo_mbox.rb DIR      writes DIR/<account>.mbox for each account
#
# tools/demo_gif requires it and calls DemoMbox.write itself.

require 'time'
require 'yaml'

module DemoMbox
  SOURCE = File.expand_path('../testdata/demo/mailbox.yml', __dir__)
  BOUNDARY = '=_demo_boundary_1'
  FROM_LINE = /^(>*From )/
  TEXT_PART = "Content-Type: text/plain; charset=utf-8\nContent-Transfer-Encoding: 8bit"

  module_function

  # Writes <dir>/<account>.mbox for each account and returns their paths by name.
  def write(dir, source = SOURCE)
    data = YAML.safe_load_file(source)
    data.fetch('accounts').to_h do |account|
      path = File.join(dir, "#{account}.mbox")
      File.write(path, render(data, account))
      [account, path]
    end
  end

  # The mbox file of one account.
  def render(data, account)
    first = data.fetch('accounts').first
    ids = message_ids(data)
    data.fetch('messages')
        .select { |msg| msg.fetch('account', first) == account }
        .map { |msg| render_message(data, msg, ids) }.join
  end

  # Every message's Message-ID by key, so a reply can name the one it answers
  # wherever that one lives.
  def message_ids(data)
    data.fetch('messages').to_h { |msg| [msg.fetch('key'), "<#{msg.fetch('key')}@demo.example>"] }
  end

  def render_message(data, msg, ids)
    date = Time.parse(data.fetch('first_day')) + (msg.fetch('at') * 3600)
    headers = base_headers(data, msg, date) + ["Message-ID: #{ids.fetch(msg.fetch('key'))}"]
    headers.concat(thread_headers(data, msg, ids))
    headers.concat(list_headers(data.dig('lists', msg['list']))) if msg['list']
    "From MAILER-DAEMON #{date.strftime('%a %b %e %H:%M:%S %Y')}\n#{entity(headers, msg)}\n"
  end

  def base_headers(data, msg, date)
    ["From: #{address(data, msg.fetch('from'))}", "To: #{recipient(data, msg)}",
     "Subject: #{encode_word(msg.fetch('subject'))}", "Date: #{date.rfc2822}", 'MIME-Version: 1.0']
  end

  def address(data, who)
    who == 'me' ? data.fetch('me') : data.fetch('people').fetch(who)
  end

  # A list message goes to the list, the user's own reply to whoever wrote
  # the message it answers, everything else to the user.
  def recipient(data, msg)
    list = data.dig('lists', msg['list'])
    return "#{list.fetch('name')} <#{list.fetch('post')}>" if list
    return data.fetch('me') unless msg.fetch('from') == 'me'

    address(data, find(data, msg.fetch('reply_to')).fetch('from'))
  end

  def find(data, key)
    data.fetch('messages').find { |msg| msg.fetch('key') == key } || raise(KeyError, "no message #{key}")
  end

  # In-Reply-To and References: the message answered and, before it, the one
  # that message answered, which is the thread's root in this mailbox.
  def thread_headers(data, msg, ids)
    parent = msg['reply_to']
    return [] unless parent

    chain = [find(data, parent)['reply_to'], parent].compact.map { |key| ids.fetch(key) }
    ["In-Reply-To: #{ids.fetch(parent)}", "References: #{chain.join(' ')}"]
  end

  def list_headers(list)
    post = list.fetch('post')
    ["List-Id: #{list.fetch('name')} <#{list.fetch('id')}>", "List-Post: <mailto:#{post}>",
     "List-Unsubscribe: <mailto:#{post.sub('@', '-leave@')}>", 'Precedence: list']
  end

  # The message's MIME body: plain text, or text plus one attachment.
  def entity(headers, msg)
    body = escape_from(msg.fetch('body'))
    attach = msg['attach']
    return "#{headers.join("\n")}\n#{TEXT_PART}\n\n#{body}" unless attach

    parts = ["--#{BOUNDARY}", TEXT_PART, '', body, "--#{BOUNDARY}",
             "Content-Type: #{attach.fetch('type')}; name=\"#{attach.fetch('name')}\"",
             "Content-Disposition: attachment; filename=\"#{attach.fetch('name')}\"",
             'Content-Transfer-Encoding: base64', '', [attach.fetch('content')].pack('m76').chomp,
             "--#{BOUNDARY}--"]
    "#{headers.join("\n")}\nContent-Type: multipart/mixed; boundary=\"#{BOUNDARY}\"\n\n#{parts.join("\n")}\n"
  end

  # A subject outside ASCII goes out as an RFC 2047 encoded word.
  def encode_word(text)
    return text if text.ascii_only?

    "=?UTF-8?B?#{[text].pack('m0')}?="
  end

  # mboxrd: a body line starting with "From " (after any '>') gains a '>'.
  def escape_from(body)
    body.gsub(FROM_LINE, '>\1')
  end
end

if $PROGRAM_NAME == __FILE__
  abort 'usage: ruby tools/demo_mbox.rb DIR' unless ARGV.length == 1
  DemoMbox.write(ARGV.fetch(0)).each_value { |path| puts path }
end
