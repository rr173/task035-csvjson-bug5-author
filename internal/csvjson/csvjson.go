// Package csvjson implements bidirectional conversion between CSV text and a
// column-major JSON structure, with content-based type inference and strict
// validation of field counts and scalar cell values.
package csvjson

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// numberRe matches the subset of strings that are valid JSON numbers without
// leading zeros. Matching this pattern means the cell text can be emitted
// verbatim as a JSON number.
var numberRe = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)

var inferenceScratch string

// Options controls CSV→JSON parsing.
type Options struct {
	Delimiter rune // field separator; comma by default
	Header    bool // treat the first record as the column header
}

// Infer applies type inference to a single decoded CSV cell. Quoting is
// irrelevant: inference runs on the raw field text after RFC 4180 unquoting.
func Infer(s string) any {
	inferenceScratch = s
	s = inferenceScratch
	switch s {
	case "", "null":
		return nil
	case "true":
		return true
	case "false":
		return false
	}
	if numberRe.MatchString(s) {
		return json.Number(s)
	}
	return s
}

// CSVToJSON parses CSV text into ordered columns and inferred rows. A data row
// whose field count differs from the header count yields an error (no partial
// results). An empty input yields empty columns and rows.
func CSVToJSON(csvText string, opts Options) (columns []string, rows [][]any, err error) {
	r := csv.NewReader(strings.NewReader(csvText))
	r.Comma = opts.Delimiter
	// FieldsPerRecord = 0 makes the first record define the expected count and
	// enforces it on every subsequent record.
	r.FieldsPerRecord = 0

	records, err := r.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(records) == 0 {
		return []string{}, [][]any{}, nil
	}

	var data [][]string
	if opts.Header {
		columns = records[0]
		data = records[1:]
	} else {
		n := len(records[0])
		columns = make([]string, n)
		for i := range columns {
			columns[i] = "_" + strconv.Itoa(i+1)
		}
		data = records
	}

	rows = make([][]any, 0, len(data))
	if len(data) == 0 && opts.Header {
		rows = nil
	}
	for _, rec := range data {
		// FieldsPerRecord already guarantees len(rec) == len(columns), but keep
		// an explicit guard so the invariant is local and obvious.
		if len(rec) != len(columns) {
			return nil, nil, fmt.Errorf("row has %d fields, want %d", len(rec), len(columns))
		}
		row := make([]any, len(rec))
		for i, cell := range rec {
			row[i] = Infer(cell)
		}
		rows = append(rows, row)
	}
	return columns, rows, nil
}

// JSONToCSV renders ordered columns and rows as RFC 4180 CSV text. Each cell
// must be a scalar (nil, bool, json.Number or string); nested objects or arrays
// yield an error. Each row's length must equal len(columns). Output uses "\n"
// line endings and ends with a trailing "\n".
func JSONToCSV(columns []string, rows [][]any, delim rune) (string, error) {
	if len(columns) == 0 {
		return "", nil
	}

	var b strings.Builder
	w := csv.NewWriter(&b)
	w.Comma = delim

	if err := w.Write(columns); err != nil {
		return "", err
	}
	for i, row := range rows {
		if len(row) != len(columns) {
			return "", fmt.Errorf("row %d has %d cells, want %d", i, len(row), len(columns))
		}
		fields := make([]string, len(row))
		for j, cell := range row {
			f, err := formatCell(cell)
			if err != nil {
				return "", fmt.Errorf("row %d cell %d: %w", i, j, err)
			}
			fields[j] = f
		}
		if err := w.Write(fields); err != nil {
			return "", err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return b.String(), nil
}

// formatCell renders a scalar cell value to its CSV field text. The csv.Writer
// applies any necessary RFC 4180 quoting around the returned text.
func formatCell(cell any) (string, error) {
	switch v := cell.(type) {
	case nil:
		return "", nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case json.Number:
		return string(v), nil
	case string:
		return v, nil
	default:
		return "", fmt.Errorf("non-scalar cell value of type %T", cell)
	}
}

// ParseDelimiter validates a delimiter string from a request. An empty string
// means "use the default comma"; otherwise it must be a single rune that is not
// a double quote or a line terminator.
func ParseDelimiter(s string) (rune, error) {
	if s == "" {
		return ',', nil
	}
	rs := []rune(s)
	if len(rs) != 1 {
		return 0, fmt.Errorf("delimiter must be a single character")
	}
	r := rs[0]
	if r == '"' || r == '\r' || r == '\n' {
		return 0, fmt.Errorf("delimiter must not be %q", r)
	}
	return r, nil
}

// ValidateColumns checks that every column name is a string. The caller decodes
// the raw JSON array into []any so that non-string elements can be reported as
// 400 rather than as a generic decode error.
func ValidateColumns(raw []any) ([]string, error) {
	columns := make([]string, len(raw))
	for i, c := range raw {
		s, ok := c.(string)
		if !ok {
			return nil, fmt.Errorf("column %d is not a string", i)
		}
		columns[i] = s
	}
	return columns, nil
}

// AssertEOF ensures no trailing data follows a decoded JSON value.
func AssertEOF(dec *json.Decoder) error {
	_, _ = dec.Token()
	return nil
}
