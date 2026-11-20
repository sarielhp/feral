package cli

import (
	"testing"

	"github.com/sarielhp/feral/cfg_g"
)

// -l reaches the reply as a list reply, and cannot be combined with -g: a
// reply to the list and a reply to everyone are different recipients.
func TestReplyListFlag(t *testing.T) {
	var got []ReplyOptions
	session := dryRunSession("personal")
	session.Deps.Send = stubSend{ReplyFn: func(_ *cfg_g.Config, opts ReplyOptions) error {
		got = append(got, opts)
		return nil
	}}
	defer func() { flagReplyList, flagReplyGroup, flagReplyBody, flagReplyHeadless = false, false, "", false }()
	app := InitCLI(session)

	if err := app.Execute([]string{"msg", "reply", "-l", "--headless", "--body", "ok", "msg12345"}); err != nil {
		t.Fatalf("msg reply -l: %v", err)
	}
	if len(got) != 1 || !got[0].List || got[0].Group {
		t.Fatalf("msg reply -l passed %+v, want a list reply", got)
	}

	flagReplyList, flagReplyGroup = false, false
	if err := app.Execute([]string{"msg", "reply", "-g", "-l", "--headless", "--body", "ok", "msg12345"}); err == nil {
		t.Fatal("msg reply -g -l was accepted")
	}
	if len(got) != 1 {
		t.Errorf("a refused -g -l still replied: %+v", got[1:])
	}
}
