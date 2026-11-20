package watcher

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sarielhp/feral/email"
)

// Notification is one new message, as the watcher announces it.
type Notification struct {
	Account string
	From    string
	Subject string
	Date    string
	ID      email.ID
}

// Notifier delivers the new mail one poll found in one account, as a single
// alert. The watcher holds one so a test can capture what would have been
// shown instead of spawning notify-send.
type Notifier interface {
	Notify(context.Context, []Notification) error
}

// notifyListed is how many messages a notification names before it says how
// many more there are.
const notifyListed = 5

// notifySendNotifier shows a desktop notification through notify-send, which is
// present on any freedesktop session and absent elsewhere. An absent binary
// returns an error, and the watcher falls back to logging the alert.
//
// The icon and the email.arrived category are the freedesktop names for new
// mail, so KDE Plasma and GNOME show a mail icon and file the alert with other
// mail in their notification history.
type notifySendNotifier struct{}

func (notifySendNotifier) Notify(ctx context.Context, news []Notification) error {
	if len(news) == 0 {
		return nil
	}
	title, body := notificationText(news)
	ctx, cancel := context.WithTimeout(ctx, deliverTimeout)
	defer cancel()
	// The leading -- keeps a subject that begins with a dash from being read as
	// an option.
	cmd := exec.CommandContext(ctx, "notify-send", "--app-name=feral", "--icon=mail-message-new",
		"--category=email.arrived", "--", title, body)
	cmd.WaitDelay = time.Second
	return cmd.Run()
}

// markupEscaper escapes what a notification body's markup gives meaning to,
// and nothing else: html.EscapeString also writes quotes as numeric entities,
// which not every notification server decodes, so "Don't" could show as
// "Don&#39;t".
var markupEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// notificationText is one account's alert: the count and the account in the
// title, and a line per message -- sender, then subject -- up to notifyListed.
// The body is escaped, because notification servers read it as markup: a
// subject such as "<b>Win</b> & more" would otherwise be rendered, or dropped
// as malformed.
func notificationText(news []Notification) (string, string) {
	title := "New mail — " + news[0].Account
	if len(news) > 1 {
		title = fmt.Sprintf("%d new messages — %s", len(news), news[0].Account)
	}
	lines := make([]string, 0, notifyListed+1)
	for i, n := range news {
		if i == notifyListed {
			lines = append(lines, fmt.Sprintf("…and %d more", len(news)-notifyListed))
			break
		}
		lines = append(lines, markupEscaper.Replace(n.From+": "+n.Subject))
	}
	return title, strings.Join(lines, "\n")
}

// runHook runs the --on-mail program with one new message in its environment.
// The message is passed as variables rather than arguments so the hook can
// ignore what it does not care about and a subject with spaces needs no
// quoting. It is killed after deliverTimeout, so a hook that blocks cannot
// stall the watcher.
func runHook(ctx context.Context, path string, n Notification) error {
	ctx, cancel := context.WithTimeout(ctx, deliverTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path)
	cmd.WaitDelay = time.Second
	cmd.Env = append(os.Environ(), hookEnv(n)...)
	return cmd.Run()
}

// hookEnv is the message as the hook's environment, under FERAL_* and, for
// hooks written before the rename, the same values under MAIL_CLI_*.
func hookEnv(n Notification) []string {
	fields := []struct{ name, value string }{
		{"ACCOUNT", n.Account}, {"FROM", n.From}, {"SUBJECT", n.Subject},
		{"DATE", n.Date}, {"ID", string(n.ID)},
	}
	env := make([]string, 0, 2*len(fields))
	for _, prefix := range []string{"FERAL_", "MAIL_CLI_"} {
		for _, f := range fields {
			env = append(env, prefix+f.name+"="+f.value)
		}
	}
	return env
}
