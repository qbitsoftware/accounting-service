// Command eb-onewriter-probe answers the Excellent Books sandbox checklist (E1–E11
// in the backend's docs/EXCELLENTBOOKS_RECEIPT_OWNERSHIP.md) against a real EB
// company — the day there is a test one. Until then every EB behaviour the
// one-writer port relies on is a documented guess with a conservative branch;
// this probe turns the guesses into observations.
//
// READ-ONLY by default:
//
//	E11  raw dump of the 5 latest receipts with every field (creator field? E5
//	     Invalid field? E10 OKFlag/Comment values)
//	E1   updates_after on IPVc, with a cursor just below those receipts
//	E2   deletes_after on IPVc (the shape of the answer)
//	E4   GET of a SerNr that does not exist
//	E9   two pages with limit/offset, and page one read again (stable? offset
//	     honoured? limit capped by the server?)
//	E3r  with -serNr: that receipt read raw and decoded
//
//	EB_BASE_URL=https://<host>:<port> EB_COMPANY=1 EB_USER=... EB_PASS=... \
//	go run ./cmd/eb-onewriter-probe -out /tmp/eb-probe [-serNr 1234] [-page 20]
//
// With -write it also WRITES — on a test company only:
//
//	E3   books one CONFIRMED receipt of EB_AMOUNT on invoice EB_INVOICE with
//	     header Comment KLUBIO-PMT-PROBE, and reads it back by id and by list
//	E6   reverses it with ReverseReceipt (Document KLUBIO-REV-<SerNr>), reading
//	     the invoice raw before, after the receipt and after the reversal
//	E7   creates a DRAFT receipt (OKFlag=0, a 120-character Comment, which also
//	     shows how much of a Comment EB keeps) and tries HTTP DELETE on it
//
//	EB_CUST=K1 EB_INVOICE=260123 EB_PAYMODE=P2 EB_AMOUNT=1.00 \
//	go run ./cmd/eb-onewriter-probe -out /tmp/eb-probe -write
//
// WRITES CANNOT BE UNDONE BY DELETING. A confirmed receipt can be neither
// edited nor deleted in Excellent Books; the only cleanup is the negative copy,
// which is exactly what E6 books. After a -write run the company holds the
// receipt and its reversal (net zero on the invoice) and, if E7's DELETE was
// refused, one draft receipt — delete that one by hand in the EB UI. Use an
// open invoice with at least EB_AMOUNT left to pay.
//
// -out <dir> writes every HTTP exchange as <n>-<step>.json (method, URL, form
// body, status, response body — never the credentials, which travel in the
// Authorization header). Each step prints one verdict line.
//
// E8 (what the bank statement import does with a transfer for an invoice a
// klubio receipt already paid) cannot be probed through the API: it needs a
// statement file imported in the EB UI. The probe prints the manual recipe.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	accounting "github.com/qbitsoftware/accounting-service"
	"github.com/qbitsoftware/accounting-service/excellentbooks"
	"github.com/shopspring/decimal"
)

const (
	registerReceipt = "IPVc"
	registerInvoice = "IVVc"
	probeTag        = "KLUBIO-PMT-PROBE"
)

// recorder is an http.RoundTripper that writes every exchange to -out, named
// by the step that made it.
type recorder struct {
	dir  string
	step string
	n    int
	base http.RoundTripper
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	var form []byte
	if req.Body != nil {
		form, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(form))
	}
	resp, err := r.base.RoundTrip(req)
	r.n++
	entry := map[string]any{"step": r.step, "method": req.Method, "url": req.URL.String()}
	if len(form) > 0 {
		entry["form"] = string(form)
	}
	if err != nil {
		entry["error"] = err.Error()
	} else {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(body))
		entry["status"] = resp.StatusCode
		var js any
		if json.Unmarshal(body, &js) == nil {
			entry["body"] = js
		} else {
			entry["body_text"] = string(body)
		}
	}
	if r.dir != "" {
		name := fmt.Sprintf("%03d-%s.json", r.n, unsafeName.ReplaceAllString(r.step, "_"))
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false) // keep the form body's & readable
		enc.SetIndent("", "  ")
		if jerr := enc.Encode(entry); jerr == nil {
			if werr := os.WriteFile(filepath.Join(r.dir, name), buf.Bytes(), 0o600); werr != nil {
				fmt.Fprintln(os.Stderr, "write", name, werr)
			}
		}
	}
	return resp, err
}

func verdict(step, status, format string, args ...any) {
	fmt.Printf("%-4s %-5s %s\n", step, status, fmt.Sprintf(format, args...))
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		fmt.Fprintf(os.Stderr, "%s is required\n", k)
		os.Exit(2)
	}
	return v
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type probe struct {
	rec *recorder
	raw *excellentbooks.Client
	sdk *accounting.Client
	ctx context.Context
}

func (p *probe) at(step string) { p.rec.step = step }

func main() {
	outDir := flag.String("out", "", "directory to write every raw HTTP exchange to (created if missing)")
	serNr := flag.String("serNr", "", "an existing receipt SerNr to read raw and decoded (E3 read)")
	write := flag.Bool("write", false, "also run the WRITE steps E3/E6/E7 (test company only; needs EB_CUST, EB_INVOICE, EB_PAYMODE, EB_AMOUNT)")
	page := flag.Int("page", 20, "page size for the E9 pagination check")
	flag.Parse()

	base := strings.TrimRight(mustEnv("EB_BASE_URL"), "/")
	company := envOr("EB_COMPANY", "1")
	user, pass := mustEnv("EB_USER"), mustEnv("EB_PASS")
	if *outDir != "" {
		if err := os.MkdirAll(*outDir, 0o700); err != nil {
			fmt.Fprintln(os.Stderr, "out dir:", err)
			os.Exit(2)
		}
	}

	rec := &recorder{dir: *outDir, base: http.DefaultTransport}
	hc := &http.Client{Transport: rec, Timeout: 90 * time.Second}
	sdk, err := accounting.NewClient(accounting.Config{
		Provider:   "excellentbooks",
		APIID:      user,
		APIKey:     pass,
		HTTPClient: hc,
		Extra:      map[string]string{"base_url": base, "company_code": company},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "client:", err)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	p := &probe{
		rec: rec,
		raw: excellentbooks.New(excellentbooks.Config{BaseURL: base, CompanyCode: company, Username: user, Password: pass, HTTPClient: hc}),
		sdk: sdk,
		ctx: ctx,
	}

	p.at("auth")
	if _, _, err := p.raw.ListCustomers(ctx, excellentbooks.ListParams{Limit: 1}); err != nil {
		verdict("auth", "FAIL", "%v (check EB_BASE_URL / EB_COMPANY / EB_USER / EB_PASS)", err)
		os.Exit(1)
	}
	verdict("auth", "OK", "%s company %s", base, company)

	latest, plainSeq := p.e11()
	cursor := cursorBelow(latest, plainSeq)
	p.e1(cursor, plainSeq, latest)
	p.e2(cursor)
	p.e4()
	p.e9(*page)
	if *serNr != "" {
		p.e3read(*serNr)
	}
	if *write {
		p.writes()
	} else {
		verdict("E3", "SKIP", "write steps E3/E6/E7 not run (add -write, on a test company only)")
	}
	verdict("E8", "MANUAL", "import a bank statement in the EB UI with a transfer naming an invoice a klubio receipt already paid "+
		"(same reference/amount); then run this probe again with -out and look in E11 for where the transfer landed: next open invoice, a CUPNr row (ettemaks), or unmatched")
}

// e11 dumps the 5 latest receipts with every field and reports the field set,
// any creator-like field, the Invalid field (E5) and OKFlag values (E10).
func (p *probe) e11() ([]map[string]any, string) {
	p.at("E11-latest")
	today := tallinnToday()
	var recs []map[string]any
	var seq string
	for _, days := range []int{60, 400, 4000} {
		data, err := p.raw.ListRaw(p.ctx, registerReceipt, excellentbooks.ListParams{
			Sort: "TransDate", Range: today.AddDate(0, 0, -days).Format("2006-01-02") + ":", Limit: 500,
		})
		if err != nil {
			verdict("E11", "FAIL", "date read: %v", err)
			return nil, ""
		}
		recs, seq = decodeRecords(data)
		if len(recs) >= 5 {
			break
		}
	}
	if len(recs) > 5 {
		recs = recs[len(recs)-5:]
	}
	header, rows := map[string]int{}, map[string]int{}
	okFlags, invalid := map[string]int{}, map[string]int{}
	for _, r := range recs {
		for k, v := range r {
			header[k]++
			switch k {
			case "OKFlag":
				okFlags[fmt.Sprint(v)]++
			case "Invalid":
				invalid[fmt.Sprintf("%T:%v", v, v)]++
			}
		}
		if rs, ok := r["rows"].([]any); ok {
			for _, row := range rs {
				if m, ok := row.(map[string]any); ok {
					for k := range m {
						rows[k]++
					}
				}
			}
		}
	}
	verdict("E11", "INFO", "%d latest receipts, data@sequence %q; header fields %v", len(recs), seq, keys(header))
	verdict("E11", "INFO", "row fields %v", keys(rows))
	creator := regexp.MustCompile(`(?i)user|creat|chang|sign|author|by$|person`)
	var hits []string
	for _, k := range append(keys(header), keys(rows)...) {
		if creator.MatchString(k) {
			hits = append(hits, k)
		}
	}
	if len(hits) > 0 {
		verdict("E11", "OK", "creator/user-like fields: %v — compare their values on an API-made and a hand-made receipt", hits)
	} else {
		verdict("E11", "NO", "no creator/user-like field in any receipt (CreatedBy stays empty)")
	}
	if len(invalid) > 0 {
		verdict("E5", "OK", "an Invalid field is present: %v — invalidate a receipt in the EB UI and re-run to see its value", invalid)
	} else {
		verdict("E5", "NO", "no Invalid field on these receipts (invalidate one in the UI and re-run with -serNr to check)")
	}
	verdict("E10", "INFO", "OKFlag values among the latest: %v (a statement-imported receipt: compare its OKFlag and Comment in the dump)", okFlags)
	return recs, seq
}

// cursorBelow is a sequence just below the oldest of the latest receipts, so
// updates_after has something to return; the plain read's sequence otherwise.
func cursorBelow(recs []map[string]any, plainSeq string) string {
	var min uint64
	for _, r := range recs {
		n, err := strconv.ParseUint(fmt.Sprint(r["@sequence"]), 10, 64)
		if err == nil && (min == 0 || n < min) {
			min = n
		}
	}
	if min > 1 {
		return strconv.FormatUint(min-1, 10)
	}
	return plainSeq
}

func (p *probe) e1(cursor, plainSeq string, latest []map[string]any) {
	if cursor == "" {
		verdict("E1", "SKIP", "no sequence to start from (E11 found no @sequence)")
		return
	}
	p.at("E1-updates_after")
	ch, err := p.sdk.Sync.ListPaymentChanges(p.ctx, accounting.PaymentChangesInput{SinceSequence: cursor})
	switch {
	case errors.Is(err, accounting.ErrChangeFeedUnsupported):
		verdict("E1", "NO", "updates_after=%s refused or not applied: %v", cursor, err)
	case err != nil:
		verdict("E1", "FAIL", "updates_after=%s: %v", cursor, err)
	default:
		seen := map[string]bool{}
		for _, r := range ch.Receipts {
			seen[r.ID] = true
		}
		found := 0
		for _, r := range latest {
			if seen[fmt.Sprint(r["SerNr"])] {
				found++
			}
		}
		verdict("E1", "OK", "updates_after=%s answered %d receipts (%d of the %d latest), data@sequence %q", cursor, len(ch.Receipts), found, len(latest), ch.Sequence)
	}
	if plainSeq != "" {
		p.at("E1-updates_after-current")
		data, err := p.raw.ListRaw(p.ctx, registerReceipt, excellentbooks.ListParams{UpdatesAfter: plainSeq, Limit: 50})
		if err != nil {
			verdict("E1", "INFO", "updates_after=<current %s>: %v", plainSeq, err)
		} else {
			recs, seq := decodeRecords(data)
			verdict("E1", "INFO", "updates_after=<current %s> answered %d records, data@sequence %q (expect 0 unless something changed meanwhile)", plainSeq, len(recs), seq)
		}
	}
}

func (p *probe) e2(cursor string) {
	for _, c := range []string{cursor, "0"} {
		if c == "" {
			continue
		}
		p.at("E2-deletes_after-" + c)
		del, err := p.raw.ListDeletedReceipts(p.ctx, c, 500)
		if err != nil {
			verdict("E2", "NO", "deletes_after=%s refused: %v", c, err)
			continue
		}
		if !del.Recognised {
			snippet := string(del.Raw)
			if len(snippet) > 300 {
				snippet = snippet[:300] + "…"
			}
			verdict("E2", "INFO", "deletes_after=%s answered a shape the SDK does not read as deletions: %s", c, snippet)
			continue
		}
		verdict("E2", "OK", "deletes_after=%s answered %d IPVc records with SerNrs, data@sequence %q: %v — check the dump: are these really deleted receipts?",
			c, len(del.Records), del.Sequence, del.Records)
	}
	if cursor != "" {
		p.at("E2-sdk")
		ch, err := p.sdk.Sync.ListPaymentChanges(p.ctx, accounting.PaymentChangesInput{SinceSequence: cursor, WithDeletions: true})
		if err != nil {
			verdict("E2", "INFO", "SDK read with deletions: %v", err)
		} else {
			verdict("E2", "INFO", "SDK takes as deleted %v, complete=%v", ch.Deleted, ch.DeletedComplete)
		}
	}
}

func (p *probe) e4() {
	const unknown = "999999999"
	p.at("E4-get-unknown")
	r, err := p.sdk.Payments.GetReceipt(p.ctx, unknown)
	switch {
	case err == nil:
		verdict("E4", "FAIL", "GET IPVc/%s answered a receipt: %+v", unknown, r)
	case accounting.IsNotFound(err):
		verdict("E4", "OK", "GET IPVc/%s is not-found (%v); see the dump for EB's exact answer", unknown, err)
	default:
		verdict("E4", "NO", "GET IPVc/%s is an error the SDK does NOT read as not-found: %v", unknown, err)
	}
}

func (p *probe) e9(size int) {
	if size < 2 {
		size = 2
	}
	read := func(step string, offset int) ([]excellentbooks.Receipt, error) {
		p.at(step)
		recs, _, err := p.raw.ListReceipts(p.ctx, excellentbooks.ListParams{Sort: "TransDate", Range: "2000-01-01:", Limit: size, Offset: offset})
		return recs, err
	}
	p1, err := read("E9-page1", 0)
	if err != nil {
		verdict("E9", "FAIL", "page 1: %v", err)
		return
	}
	p2, err := read("E9-page2", size-1)
	if err != nil {
		verdict("E9", "FAIL", "page 2 (offset %d): %v", size-1, err)
		return
	}
	again, err := read("E9-page1-again", 0)
	if err != nil {
		verdict("E9", "FAIL", "page 1 again: %v", err)
		return
	}
	ids := func(rs []excellentbooks.Receipt) string {
		s := make([]string, len(rs))
		for i, r := range rs {
			s[i] = r.SerNr
		}
		return strings.Join(s, ",")
	}
	switch {
	case len(p1) < size && len(p2) > 1:
		verdict("E9", "NO", "limit=%d answered %d but offset %d answered %d more: the server caps limit below %d — the SDK's page of 500 would stop early",
			size, len(p1), size-1, len(p2), size)
	case len(p1) < size:
		verdict("E9", "INFO", "only %d receipts in the register; use a smaller -page to test paging", len(p1))
	case len(p2) == 0 || p2[0].SerNr != p1[len(p1)-1].SerNr:
		first := "nothing"
		if len(p2) > 0 {
			first = p2[0].SerNr
		}
		verdict("E9", "NO", "offset %d starts with %s, not page 1's last %s: offset is not 0-based record skipping", size-1, first, p1[len(p1)-1].SerNr)
	default:
		verdict("E9", "OK", "offset is 0-based: page 2 starts on page 1's last receipt %s", p1[len(p1)-1].SerNr)
	}
	if ids(p1) == ids(again) {
		verdict("E9", "OK", "page 1 read twice in the same order (%d receipts)", len(p1))
	} else {
		verdict("E9", "NO", "page 1 changed between two reads:\n      %s\n      %s", ids(p1), ids(again))
	}
}

func (p *probe) e3read(serNr string) {
	p.at("E3r-raw-" + serNr)
	if _, err := p.raw.GetRaw(p.ctx, registerReceipt, serNr); err != nil {
		verdict("E3r", "FAIL", "GET IPVc/%s: %v", serNr, err)
		return
	}
	p.at("E3r-sdk-" + serNr)
	r, err := p.sdk.Payments.GetReceipt(p.ctx, serNr)
	if err != nil {
		verdict("E3r", "FAIL", "decode %s: %v", serNr, err)
		return
	}
	verdict("E3r", "INFO", "%s", describe(r))
}

func (p *probe) writes() {
	cust, inv := mustEnv("EB_CUST"), mustEnv("EB_INVOICE")
	payMode := mustEnv("EB_PAYMODE")
	amount, err := decimal.NewFromString(mustEnv("EB_AMOUNT"))
	if err != nil || !amount.IsPositive() {
		fmt.Fprintln(os.Stderr, "EB_AMOUNT must be a positive number")
		os.Exit(2)
	}
	today := tallinnToday()

	p.at("E6-invoice-before")
	before := p.invoiceRaw(inv)

	// E3: a confirmed receipt carrying the tag, read back two ways.
	p.at("E3-create")
	created, err := p.sdk.Payments.CreateReceipt(p.ctx, accounting.CreatePaymentInput{
		CustomerCode: cust,
		InvoiceNo:    inv,
		PaymentDate:  today,
		Amount:       amount,
		BankID:       payMode,
		Document:     probeTag,
		Comment:      "klubio eb-onewriter-probe",
	})
	if err != nil {
		verdict("E3", "FAIL", "create a confirmed receipt on %s: %v", inv, err)
		return
	}
	if created.ID != "" {
		verdict("E3", "OK", "created receipt SerNr %s, read off the POST answer", created.ID)
	} else {
		verdict("E3", "NO", "the POST answer named no SerNr; looking the receipt up by its Comment")
	}
	p.at("E3-readback-list")
	listed, err := p.sdk.Payments.ListInvoiceReceipts(p.ctx, accounting.ListInvoiceReceiptsInput{CustomerID: cust, InvoiceID: inv, Since: today.AddDate(0, 0, -1)})
	if err != nil {
		verdict("E3", "FAIL", "list the invoice's receipts: %v", err)
	}
	var fromList *accounting.Receipt
	for i := range listed {
		if (created.ID != "" && listed[i].ID == created.ID) || (created.ID == "" && listed[i].Document == probeTag) {
			fromList = &listed[i]
		}
	}
	id := created.ID
	switch {
	case fromList == nil:
		verdict("E3", "NO", "the new receipt is not in the invoice's receipt list (%d receipts listed)", len(listed))
	case fromList.Document == probeTag:
		verdict("E3", "OK", "list read keeps the header Comment: %s", describe(fromList))
		id = fromList.ID
	default:
		verdict("E3", "NO", "list read lost the header Comment (got %q): %s", fromList.Document, describe(fromList))
		id = fromList.ID
	}
	if id == "" {
		verdict("E6", "SKIP", "no SerNr for the new receipt — reverse it by hand in the EB UI")
		return
	}
	p.at("E3-readback-id")
	byID, err := p.sdk.Payments.GetReceipt(p.ctx, id)
	switch {
	case err != nil:
		verdict("E3", "FAIL", "read %s by id: %v", id, err)
	case byID.Document == probeTag:
		verdict("E3", "OK", "by-id read keeps the header Comment: %s", describe(byID))
	default:
		verdict("E3", "NO", "by-id read lost the header Comment (got %q)", byID.Document)
	}

	p.at("E6-invoice-after-receipt")
	afterReceipt := p.invoiceRaw(inv)

	// E6: the negative copy.
	p.at("E6-reverse")
	rev, err := p.sdk.Payments.ReverseReceipt(p.ctx, accounting.ReverseReceiptInput{ID: id, Date: today, Document: "KLUBIO-REV-" + id})
	if err != nil {
		verdict("E6", "NO", "the negative copy of %s was refused: %v — reverse it by hand in the EB UI", id, err)
	} else {
		verdict("E6", "OK", "reversal accepted: SerNr %q, amount %s", rev.ID, rev.Amount)
		p.at("E6-readback")
		net := decimal.Zero
		all, err := p.sdk.Payments.ListInvoiceReceipts(p.ctx, accounting.ListInvoiceReceiptsInput{CustomerID: cust, InvoiceID: inv, Since: today.AddDate(0, 0, -1)})
		if err != nil {
			verdict("E6", "FAIL", "list after reversal: %v", err)
		}
		for _, r := range all {
			if r.ID == id || r.ID == rev.ID {
				net = net.Add(r.AmountOn(inv))
				verdict("E6", "INFO", "%s", describe(&r))
			}
		}
		verdict("E6", "INFO", "receipt + reversal net on %s: %s (want 0)", inv, net)
	}
	p.at("E6-invoice-after-reversal")
	afterReversal := p.invoiceRaw(inv)
	verdict("E6", "INFO", "invoice fields changed by the receipt: %v", diffKeys(before, afterReceipt))
	verdict("E6", "INFO", "invoice fields changed by the reversal: %v (re-opened? confirm the open amount in the EB UI if none is listed)", diffKeys(afterReceipt, afterReversal))

	// E7: a draft, then DELETE.
	long := "KLUBIO-PROBE-DRAFT " + strings.Repeat("0123456789", 11)
	p.at("E7-draft-create")
	draft, err := p.raw.PostReceipt(p.ctx, map[string]string{
		"set_field.TransDate":       today.Format("2006-01-02"),
		"set_field.OKFlag":          "0",
		"set_field.PayMode":         payMode,
		"set_field.Comment":         long,
		"set_row_field.0.stp":       "1",
		"set_row_field.0.InvoiceNr": inv,
		"set_row_field.0.CustCode":  cust,
		"set_row_field.0.RecVal":    amount.String(),
		"set_row_field.0.PayDate":   today.Format("2006-01-02"),
	})
	if err != nil {
		verdict("E7", "FAIL", "create a draft: %v", err)
		return
	}
	if draft == nil || draft.SerNr == "" {
		verdict("E7", "NO", "the draft's answer named no SerNr — find it by Comment %q in the UI and delete it", long[:20])
		return
	}
	p.at("E7-draft-readback")
	got, err := p.raw.GetReceiptRecords(p.ctx, draft.SerNr)
	if err == nil && len(got) > 0 {
		verdict("E3", "INFO", "a %d-character header Comment came back as %d characters (OKFlag %q)", len(long), len(got[0].Comment), got[0].OKFlag)
	}
	p.at("E7-delete")
	if _, err := p.raw.DeleteRaw(p.ctx, registerReceipt, draft.SerNr); err != nil {
		verdict("E7", "NO", "DELETE IPVc/%s refused: %v — delete the draft by hand in the EB UI", draft.SerNr, err)
		return
	}
	p.at("E7-get-after-delete")
	if _, err := p.sdk.Payments.GetReceipt(p.ctx, draft.SerNr); accounting.IsNotFound(err) {
		verdict("E7", "OK", "the draft %s was deleted (GET is not-found afterwards)", draft.SerNr)
	} else {
		verdict("E7", "NO", "DELETE answered success but GET %s still answers: %v", draft.SerNr, err)
	}
}

func (p *probe) invoiceRaw(inv string) map[string]any {
	data, err := p.raw.GetRaw(p.ctx, registerInvoice, inv)
	if err != nil {
		verdict("E6", "INFO", "GET IVVc/%s: %v", inv, err)
		return nil
	}
	recs, _ := decodeRegister(data, registerInvoice)
	if len(recs) == 0 {
		return nil
	}
	return recs[0]
}

func describe(r *accounting.Receipt) string {
	var rows []string
	for _, row := range r.Rows {
		rows = append(rows, fmt.Sprintf("%s %s %s %q", row.Type, row.DocID, row.Amount, row.Description))
	}
	return fmt.Sprintf("SerNr %s date %s PayMode %s Document %q draft=%v void=%v seq=%s amount=%s extras=%v decodeErr=%q rows=[%s]",
		r.ID, r.Date.Format("2006-01-02"), r.AccountName, r.Document, r.Draft, r.Void, r.Sequence, r.Amount, r.HasExtras, r.DecodeError, strings.Join(rows, "; "))
}

// decodeRecords returns the IPVc records of a data payload as generic maps,
// and its data@sequence.
func decodeRecords(data json.RawMessage) ([]map[string]any, string) {
	return decodeRegister(data, registerReceipt)
}

func decodeRegister(data json.RawMessage, register string) ([]map[string]any, string) {
	var env map[string]json.RawMessage
	if json.Unmarshal(data, &env) != nil {
		return nil, ""
	}
	var seq string
	if raw, ok := env["@sequence"]; ok {
		var s any
		if json.Unmarshal(raw, &s) == nil {
			seq = fmt.Sprint(s)
		}
	}
	var list []map[string]any
	if json.Unmarshal(env[register], &list) != nil {
		var one map[string]any
		if json.Unmarshal(env[register], &one) == nil && one != nil {
			list = []map[string]any{one}
		}
	}
	return list, seq
}

func keys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func diffKeys(a, b map[string]any) []string {
	var out []string
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	for k := range seen {
		if k == "rows" {
			continue
		}
		if fmt.Sprint(a[k]) != fmt.Sprint(b[k]) {
			out = append(out, fmt.Sprintf("%s: %v → %v", k, a[k], b[k]))
		}
	}
	sort.Strings(out)
	return out
}

func tallinnToday() time.Time {
	loc, err := time.LoadLocation("Europe/Tallinn")
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}
