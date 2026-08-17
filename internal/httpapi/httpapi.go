// Package httpapi exposes the CSV/JSON conversion service over HTTP.
package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"task035-csvjson/internal/csvjson"
)

// API is a stateless converter: every request is handled independently.
type API struct{}

// New creates an API.
func New() *API { return &API{} }

// Handler returns the HTTP handler serving all routes.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealth)
	mux.HandleFunc("POST /csv2json", a.handleCSV2JSON)
	mux.HandleFunc("POST /json2csv", a.handleJSON2CSV)
	return mux
}

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) handleCSV2JSON(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()

	// Pointer fields distinguish "absent" from the zero value: a missing csv
	// field is a 400, while an explicit "" is a valid (empty) CSV.
	var req struct {
		CSV       *string `json:"csv"`
		Delimiter string  `json:"delimiter"`
		Header    *bool   `json:"header"`
	}
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid json: "+err.Error()))
		return
	}
	if err := csvjson.AssertEOF(dec); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	if req.CSV == nil {
		writeJSON(w, http.StatusBadRequest, errBody("missing csv"))
		return
	}
	delim, err := csvjson.ParseDelimiter(req.Delimiter)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	header := true
	if req.Header != nil {
		header = *req.Header
	}

	columns, rows, err := csvjson.CSVToJSON(*req.CSV, csvjson.Options{Delimiter: delim, Header: header})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"columns": columns, "rows": rows})
}

func (a *API) handleJSON2CSV(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()

	var req struct {
		Columns   *[]any   `json:"columns"`
		Rows      *[][]any `json:"rows"`
		Delimiter string   `json:"delimiter"`
	}
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid json: "+err.Error()))
		return
	}
	if err := csvjson.AssertEOF(dec); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	if req.Columns == nil {
		writeJSON(w, http.StatusBadRequest, errBody("missing columns"))
		return
	}
	if req.Rows == nil {
		writeJSON(w, http.StatusBadRequest, errBody("missing rows"))
		return
	}
	delim, err := csvjson.ParseDelimiter(req.Delimiter)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	columns, err := csvjson.ValidateColumns(*req.Columns)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}

	csvText, err := csvjson.JSONToCSV(columns, *req.Rows, delim)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"csv": csvText})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func errBody(msg string) map[string]any {
	return map[string]any{"error": msg}
}

// Compile-time guard that io is referenced (kept for clarity of intent: the
// decoder EOF contract relies on io.EOF).
var _ = io.EOF
