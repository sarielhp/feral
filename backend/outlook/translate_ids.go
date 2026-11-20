package outlook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/report"
)

const (
	translateIDsURL = "https://graph.microsoft.com/v1.0/me/translateExchangeIds"
	// translateBatch is the most IDs Graph translates in one request.
	translateBatch = 1000
)

// IDTranslator maps the account's default Graph message IDs to immutable ones,
// for migrating a cache keyed by the former (see package outlookids).
type IDTranslator struct {
	client *http.Client
}

// NewIDTranslator builds a translator for the account. It never asks for
// immutable IDs itself: the IDs it is given are the default ones.
func NewIDTranslator(acc cfg_acc.AccountConfig, config *cfg_g.Config, rep report.Reporter) *IDTranslator {
	return &IDTranslator{client: getOutlookClient(config, acc.Name, report.Or(rep), false)}
}

// Translate returns the immutable ID of each of ids. An ID Graph cannot
// translate -- one whose message is gone, or that is not a message ID -- is
// left out; a failed request is an error, and no partial result is returned.
func (t *IDTranslator) Translate(ctx context.Context, ids []email.ID) (map[email.ID]email.ID, error) {
	out := make(map[email.ID]email.ID, len(ids))
	for start := 0; start < len(ids); start += translateBatch {
		end := min(start+translateBatch, len(ids))
		if err := t.translateBatch(ctx, ids[start:end], out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (t *IDTranslator) translateBatch(ctx context.Context, ids []email.ID, out map[email.ID]email.ID) error {
	input := make([]string, len(ids))
	for i, id := range ids {
		input[i] = string(id)
	}
	body, err := json.Marshal(map[string]any{
		"inputIds":     input,
		"sourceIdType": "restId",
		"targetIdType": "restImmutableEntryId",
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, translateIDsURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("translateExchangeIds: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("translateExchangeIds: status %d: %s", resp.StatusCode, readErrorBody(resp.Body))
	}
	var result struct {
		Value []struct {
			SourceID string `json:"sourceId"`
			TargetID string `json:"targetId"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("translateExchangeIds: decoding the answer: %w", err)
	}
	for _, v := range result.Value {
		if v.SourceID != "" && v.TargetID != "" {
			out[email.ID(v.SourceID)] = email.ID(v.TargetID)
		}
	}
	return nil
}

// toImmutable translates ids, keeping any Graph could not translate as it
// was: it may already be immutable, and dropping it would lose a result.
func (t *IDTranslator) toImmutable(ctx context.Context, ids []email.ID) ([]email.ID, error) {
	translated, err := t.Translate(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]email.ID, len(ids))
	for i, id := range ids {
		out[i] = id
		if newID, ok := translated[id]; ok {
			out[i] = newID
		}
	}
	return out, nil
}

// verifySampleURLs are the listings Verify compares, tried in turn until one
// has messages: the inbox, then the whole mailbox.
var verifySampleURLs = []string{
	"https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages?$top=10&$orderby=receivedDateTime%20desc&$select=id",
	"https://graph.microsoft.com/v1.0/me/messages?$top=10&$orderby=receivedDateTime%20desc&$select=id",
}

// Verify proves on this account that translation agrees with Graph's own
// immutable IDs: it lists the same messages twice, with default and with
// immutable IDs, and checks that translating the first list gives the second,
// byte for byte. A mismatch would carry every decision onto IDs no listing
// returns, so a migration must not proceed past it. An account with no
// messages has nothing to compare and passes.
func (t *IDTranslator) Verify(ctx context.Context) error {
	for _, u := range verifySampleURLs {
		plain, err := t.listIDs(ctx, u, false)
		if err != nil {
			return err
		}
		if len(plain) == 0 {
			continue
		}
		immutable, err := t.listIDs(ctx, u, true)
		if err != nil {
			return err
		}
		translated, err := t.Translate(ctx, plain)
		if err != nil {
			return err
		}
		if len(immutable) != len(plain) {
			return fmt.Errorf("verifying translation: the two listings differ in length (%d, %d); a message arrived meanwhile -- try again", len(plain), len(immutable))
		}
		for i, id := range plain {
			if translated[id] != immutable[i] {
				return fmt.Errorf("verifying translation: %s translates to %q but Graph lists it as %q; this account's IDs cannot be migrated this way", id, translated[id], immutable[i])
			}
		}
		return nil
	}
	return nil
}

func (t *IDTranslator) listIDs(ctx context.Context, url string, immutable bool) ([]email.ID, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	if immutable {
		req.Header.Set("Prefer", preferImmutableIDs)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("listing messages: status %d: %s", resp.StatusCode, readErrorBody(resp.Body))
	}
	var res struct {
		Value []struct {
			ID string `json:"id"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	ids := make([]email.ID, len(res.Value))
	for i, v := range res.Value {
		ids[i] = email.ID(v.ID)
	}
	return ids, nil
}
