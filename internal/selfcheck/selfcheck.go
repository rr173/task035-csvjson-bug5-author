// Package selfcheck runs the CSV/JSON service against itself through in-process
// HTTP servers. Each check gets a fresh server so no state can leak between
// checks, even though the service itself is stateless.
package selfcheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"

	"task035-csvjson/internal/httpapi"
)

type client struct {
	base string
	c    *http.Client
	srv  *httptest.Server
}

func newClient() *client {
	srv := httptest.NewServer(httpapi.New().Handler())
	return &client{base: srv.URL, c: srv.Client(), srv: srv}
}

func (c *client) close() { c.srv.Close() }

// post sends body as JSON and returns the HTTP status plus the decoded
// response. Numbers are decoded as json.Number so comparisons are exact.
func (c *client) post(path string, body any) (int, map[string]any) {
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, c.base+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.c.Do(req)
	if err != nil {
		return 0, map[string]any{"error": err.Error()}
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	_ = dec.Decode(&out)
	return resp.StatusCode, out
}

// csv2jsonResult is a typed view of the /csv2json response for exact comparison.
type csv2jsonResult struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
	Error   string   `json:"error"`
}

func (c *client) csv2json(body any) (int, *csv2jsonResult) {
	code, m := c.post("/csv2json", body)
	r := &csv2jsonResult{}
	if v, ok := m["columns"].([]any); ok {
		r.Columns = make([]string, len(v))
		for i, x := range v {
			r.Columns[i], _ = x.(string)
		}
	}
	if v, ok := m["rows"].([]any); ok {
		// "rows" present (even if empty []); distinguish from absent so an
		// empty-CSV response compares equal to [][]any{} rather than nil.
		r.Rows = [][]any{}
		for _, row := range v {
			if cells, ok := row.([]any); ok {
				r.Rows = append(r.Rows, cells)
			}
		}
	}
	if e, ok := m["error"].(string); ok {
		r.Error = e
	}
	return code, r
}

func (c *client) json2csv(body any) (int, string, string) {
	code, m := c.post("/json2csv", body)
	csv, _ := m["csv"].(string)
	errMsg, _ := m["error"].(string)
	return code, csv, errMsg
}

func eq(label string, got, want any) error {
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("%s mismatch:\n  got:  %#v\n  want: %#v", label, got, want)
	}
	return nil
}

func jnum(s string) json.Number { return json.Number(s) }

// Run executes all self-checks. Returns 0 on success, 1 on the first failure.
func Run() int {
	checks := []struct {
		name string
		fn   func() error
	}{
		{"healthz", checkHealthz},
		{"type_inference", checkTypeInference},
		{"quoted_comma", checkQuotedComma},
		{"quoted_newline", checkQuotedNewline},
		{"quoted_escape", checkQuotedEscape},
		{"quote_does_not_affect_inference", checkQuoteInference},
		{"header_false", checkHeaderFalse},
		{"ragged_row_400", checkRaggedRow},
		{"missing_csv_400", checkMissingCSV},
		{"invalid_delimiter_400", checkInvalidDelimiter},
		{"empty_csv", checkEmptyCSV},
		{"json2csv_basic", checkJSON2CSVBasic},
		{"json2csv_quoting", checkJSON2CSVQuoting},
		{"json2csv_ragged_400", checkJSON2CSVRagged},
		{"json2csv_nonscalar_400", checkJSON2CSVNonscalar},
		{"json2csv_nonstring_column_400", checkJSON2CSVNonstringColumn},
		{"roundtrip_unambiguous", checkRoundtrip},
		{"roundtrip_special_chars", checkRoundtripSpecial},
	}
	for _, c := range checks {
		if err := c.fn(); err != nil {
			fmt.Printf("FAIL %s: %v\n", c.name, err)
			return 1
		}
		fmt.Printf("ok %s\n", c.name)
	}
	fmt.Println("OK")
	return 0
}

func checkHealthz() error {
	c := newClient()
	defer c.close()
	resp, err := c.c.Get(c.base + "/healthz")
	if err != nil {
		return fmt.Errorf("healthz: %v", err)
	}
	defer resp.Body.Close()
	if err := eq("healthz status", resp.StatusCode, http.StatusOK); err != nil {
		return err
	}
	var out struct {
		OK bool `json:"ok"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	return eq("healthz body", out.OK, true)
}

// checkTypeInference verifies null/bool/number/string inference on plain cells.
func checkTypeInference() error {
	c := newClient()
	defer c.close()
	code, r := c.csv2json(map[string]any{
		"csv": "name,age,score,active,note\nAlice,30,3.5,true,null\nBob,,0,false,hello",
	})
	if err := eq("status", code, http.StatusOK); err != nil {
		return err
	}
	want := &csv2jsonResult{
		Columns: []string{"name", "age", "score", "active", "note"},
		Rows: [][]any{
			{"Alice", jnum("30"), jnum("3.5"), true, nil},
			{"Bob", nil, jnum("0"), false, "hello"},
		},
	}
	return eq("csv2json", r, want)
}

// checkQuotedComma: a quoted field containing a comma is one cell.
func checkQuotedComma() error {
	c := newClient()
	defer c.close()
	code, r := c.csv2json(map[string]any{
		"csv": "name,city\nAlice,\"Beijing, China\"",
	})
	if err := eq("status", code, http.StatusOK); err != nil {
		return err
	}
	want := &csv2jsonResult{
		Columns: []string{"name", "city"},
		Rows:    [][]any{{"Alice", "Beijing, China"}},
	}
	return eq("csv2json", r, want)
}

// checkQuotedNewline: a quoted field spanning two lines is one cell.
func checkQuotedNewline() error {
	c := newClient()
	defer c.close()
	code, r := c.csv2json(map[string]any{
		"csv": "name,note\nAlice,\"line1\nline2\"",
	})
	if err := eq("status", code, http.StatusOK); err != nil {
		return err
	}
	want := &csv2jsonResult{
		Columns: []string{"name", "note"},
		Rows:    [][]any{{"Alice", "line1\nline2"}},
	}
	return eq("csv2json", r, want)
}

// checkQuotedEscape: doubled quotes inside a quoted field decode to one quote.
func checkQuotedEscape() error {
	c := newClient()
	defer c.close()
	code, r := c.csv2json(map[string]any{
		"csv": "name,quote\nAlice,\"say \"\"hi\"\"\"",
	})
	if err := eq("status", code, http.StatusOK); err != nil {
		return err
	}
	want := &csv2jsonResult{
		Columns: []string{"name", "quote"},
		Rows:    [][]any{{"Alice", `say "hi"`}},
	}
	return eq("csv2json", r, want)
}

// checkQuoteInference: quoting does not affect type inference — a quoted "42"
// is still inferred as the number 42.
func checkQuoteInference() error {
	c := newClient()
	defer c.close()
	code, r := c.csv2json(map[string]any{
		"csv": "v\n\"42\"\n\"true\"\n\"null\"",
	})
	if err := eq("status", code, http.StatusOK); err != nil {
		return err
	}
	want := &csv2jsonResult{
		Columns: []string{"v"},
		Rows: [][]any{
			{jnum("42")},
			{true},
			{nil},
		},
	}
	return eq("csv2json", r, want)
}

// checkHeaderFalse: with header=false, columns are _1.._N and all rows are data.
func checkHeaderFalse() error {
	c := newClient()
	defer c.close()
	code, r := c.csv2json(map[string]any{
		"csv":    "Alice,30\nBob,25",
		"header": false,
	})
	if err := eq("status", code, http.StatusOK); err != nil {
		return err
	}
	want := &csv2jsonResult{
		Columns: []string{"_1", "_2"},
		Rows: [][]any{
			{"Alice", jnum("30")},
			{"Bob", jnum("25")},
		},
	}
	return eq("csv2json", r, want)
}

// checkRaggedRow: a data row with the wrong field count is a 400.
func checkRaggedRow() error {
	c := newClient()
	defer c.close()
	code, r := c.csv2json(map[string]any{
		"csv": "a,b,c\n1,2,3\n4,5",
	})
	if err := eq("status", code, http.StatusBadRequest); err != nil {
		return err
	}
	if r.Columns != nil || r.Rows != nil {
		return fmt.Errorf("expected no columns/rows on error, got columns=%v rows=%v", r.Columns, r.Rows)
	}
	return nil
}

// checkMissingCSV: omitting the csv field is a 400.
func checkMissingCSV() error {
	c := newClient()
	defer c.close()
	code, r := c.csv2json(map[string]any{"delimiter": ","})
	if err := eq("status", code, http.StatusBadRequest); err != nil {
		return err
	}
	return eq("error", r.Error == "", false)
}

// checkInvalidDelimiter: a multi-character or forbidden delimiter is a 400.
func checkInvalidDelimiter() error {
	c := newClient()
	defer c.close()
	for _, d := range []string{",,", "\"", "\n"} {
		code, _ := c.csv2json(map[string]any{"csv": "a,b\n1,2", "delimiter": d})
		if err := eq("delimiter="+d, code, http.StatusBadRequest); err != nil {
			c.close()
			return err
		}
	}
	return nil
}

// checkEmptyCSV: an empty CSV yields empty columns and rows.
func checkEmptyCSV() error {
	c := newClient()
	defer c.close()
	code, r := c.csv2json(map[string]any{"csv": ""})
	if err := eq("status", code, http.StatusOK); err != nil {
		return err
	}
	return eq("csv2json", r, &csv2jsonResult{Columns: []string{}, Rows: [][]any{}})
}

// checkJSON2CSVBasic: header + rows render with correct scalar formatting.
func checkJSON2CSVBasic() error {
	c := newClient()
	defer c.close()
	code, csv, _ := c.json2csv(map[string]any{
		"columns": []string{"name", "age", "active"},
		"rows":    [][]any{{"Alice", jnum("30"), true}, {"Bob", nil, false}},
	})
	if err := eq("status", code, http.StatusOK); err != nil {
		return err
	}
	want := "name,age,active\nAlice,30,true\nBob,,false\n"
	return eq("csv", csv, want)
}

// checkJSON2CSVQuoting: fields containing comma/newline/quote are quoted and
// embedded quotes are doubled.
func checkJSON2CSVQuoting() error {
	c := newClient()
	defer c.close()
	code, csv, _ := c.json2csv(map[string]any{
		"columns": []string{"plain", "comma", "newline", "quote"},
		"rows": [][]any{{
			"hi", "a,b", "x\ny", `a"b`,
		}},
	})
	if err := eq("status", code, http.StatusOK); err != nil {
		return err
	}
	want := "plain,comma,newline,quote\nhi,\"a,b\",\"x\ny\",\"a\"\"b\"\n"
	return eq("csv", csv, want)
}

// checkJSON2CSVRagged: a row whose length differs from columns is a 400.
func checkJSON2CSVRagged() error {
	c := newClient()
	defer c.close()
	code, _, errMsg := c.json2csv(map[string]any{
		"columns": []string{"a", "b"},
		"rows":    [][]any{{"1", "2", "3"}},
	})
	if err := eq("status", code, http.StatusBadRequest); err != nil {
		return err
	}
	return eq("error", errMsg == "", false)
}

// checkJSON2CSVNonscalar: a nested object/array cell is a 400.
func checkJSON2CSVNonscalar() error {
	c := newClient()
	defer c.close()
	code, _, _ := c.json2csv(map[string]any{
		"columns": []string{"a"},
		"rows":    [][]any{{map[string]any{"x": 1}}},
	})
	if err := eq("object status", code, http.StatusBadRequest); err != nil {
		return err
	}
	code, _, _ = c.json2csv(map[string]any{
		"columns": []string{"a"},
		"rows":    [][]any{{[]any{1, 2}}},
	})
	return eq("array status", code, http.StatusBadRequest)
}

// checkJSON2CSVNonstringColumn: a non-string column name is a 400.
func checkJSON2CSVNonstringColumn() error {
	c := newClient()
	defer c.close()
	code, _, _ := c.json2csv(map[string]any{
		"columns": []any{"a", 1},
		"rows":    [][]any{},
	})
	return eq("status", code, http.StatusBadRequest)
}

// checkRoundtrip: csv2json(json2csv(X)) reproduces X for an unambiguous X
// (no string value parses as null/bool/number).
func checkRoundtrip() error {
	c := newClient()
	defer c.close()
	x := map[string]any{
		"columns": []string{"name", "age", "active"},
		"rows": [][]any{
			{"Alice", jnum("30"), true},
			{"Bob", nil, false},
		},
	}
	code, csv, _ := c.json2csv(x)
	if err := eq("json2csv status", code, http.StatusOK); err != nil {
		return err
	}
	code2, r := c.csv2json(map[string]any{"csv": csv})
	if err := eq("csv2json status", code2, http.StatusOK); err != nil {
		return err
	}
	want := &csv2jsonResult{
		Columns: []string{"name", "age", "active"},
		Rows: [][]any{
			{"Alice", jnum("30"), true},
			{"Bob", nil, false},
		},
	}
	return eq("roundtrip", r, want)
}

// checkRoundtripSpecial: round-trip preserves strings containing comma,
// newline and embedded quotes.
func checkRoundtripSpecial() error {
	c := newClient()
	defer c.close()
	x := map[string]any{
		"columns": []string{"k", "v"},
		"rows": [][]any{
			{"a,b", "x\ny"},
			{`a"b`, "plain"},
		},
	}
	code, csv, _ := c.json2csv(x)
	if err := eq("json2csv status", code, http.StatusOK); err != nil {
		return err
	}
	code2, r := c.csv2json(map[string]any{"csv": csv})
	if err := eq("csv2json status", code2, http.StatusOK); err != nil {
		return err
	}
	want := &csv2jsonResult{
		Columns: []string{"k", "v"},
		Rows: [][]any{
			{"a,b", "x\ny"},
			{`a"b`, "plain"},
		},
	}
	return eq("roundtrip special", r, want)
}
