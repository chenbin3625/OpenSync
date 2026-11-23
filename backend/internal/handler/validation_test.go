package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"opensync/internal/mapper"
	"opensync/internal/msg"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func TestParseRequiredIDRejectsInvalidValues(t *testing.T) {
	for _, input := range []string{"", "abc", "0", "-1"} {
		if _, err := parseRequiredID(input, "id"); err == nil {
			t.Fatalf("parseRequiredID(%q) returned nil error, want error", input)
		}
	}
}

func TestParseRequiredIDAcceptsPositiveInteger(t *testing.T) {
	id, err := parseRequiredID("42", "id")
	if err != nil {
		t.Fatalf("parseRequiredID() error: %v", err)
	}
	if id != 42 {
		t.Fatalf("parseRequiredID() = %d, want 42", id)
	}
}

func TestParseEnableValueRejectsMissingOrInvalidValues(t *testing.T) {
	for _, input := range []interface{}{nil, "", "abc", 2, -1, 1.5} {
		if _, err := parseEnableValue(input); err == nil {
			t.Fatalf("parseEnableValue(%#v) returned nil error, want error", input)
		}
	}
}

func TestParseEnableValueAcceptsExplicitBooleanOrBinaryValues(t *testing.T) {
	tests := []struct {
		input interface{}
		want  int
	}{
		{true, 1},
		{false, 0},
		{1, 1},
		{0, 0},
		{"1", 1},
		{"0", 0},
		{"true", 1},
		{"false", 0},
	}

	for _, tt := range tests {
		got, err := parseEnableValue(tt.input)
		if err != nil {
			t.Fatalf("parseEnableValue(%#v) error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("parseEnableValue(%#v) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestUpdateNotifyRejectsUnknownPayloadShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/svr/notify", UpdateNotify)

	req := httptest.NewRequest(http.MethodPut, "/svr/notify", strings.NewReader(`{"unexpected":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), `"code":500`) {
		t.Fatalf("response = %s, want error envelope", w.Body.String())
	}
}

func TestUpdateNotifyHandlesLargeNumericNotifyID(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE notify(
		id integer primary key,
		enable integer,
		method integer,
		params text,
		createTime integer DEFAULT 1
	)`); err != nil {
		t.Fatalf("create notify: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO notify(id, enable, method, params) VALUES (1000000, 0, 1, '{"sendKey":"k"}')`); err != nil {
		t.Fatalf("insert notify: %v", err)
	}
	restoreDB := mapper.SetDBForTest(db)
	t.Cleanup(restoreDB)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/svr/notify", UpdateNotify)

	req := httptest.NewRequest(http.MethodPut, "/svr/notify", strings.NewReader(`{"notifyId":1000000,"enable":1}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"code":200`) {
		t.Fatalf("status=%d body=%s, want 200 success for notifyId=1000000", w.Code, w.Body.String())
	}
	var enable int
	if err := db.QueryRow("SELECT enable FROM notify WHERE id=1000000").Scan(&enable); err != nil {
		t.Fatalf("query notify: %v", err)
	}
	if enable != 1 {
		t.Fatalf("enable = %d, want 1", enable)
	}
}

func TestHandlersRejectNullJSONBodyWithoutPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/svr/alist", AddAlist)
	router.PUT("/svr/alist", UpdateAlist)
	router.POST("/svr/job", AddJob)
	router.PUT("/svr/job", UpdateJob)
	router.POST("/svr/notify", AddNotify)
	router.POST("/svr/notify/test", TestNotify)
	router.PUT("/svr/notify", UpdateNotify)
	router.PUT("/svr/system/config", UpdateSystemConfig)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/svr/alist"},
		{http.MethodPut, "/svr/alist"},
		{http.MethodPost, "/svr/job"},
		{http.MethodPut, "/svr/job"},
		{http.MethodPost, "/svr/notify"},
		{http.MethodPost, "/svr/notify/test"},
		{http.MethodPut, "/svr/notify"},
		{http.MethodPut, "/svr/system/config"},
	}

	for _, r := range routes {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.path, strings.NewReader("null"))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}
			if !strings.Contains(w.Body.String(), `"code":500`) || !strings.Contains(w.Body.String(), msg.LostPart) {
				t.Fatalf("body = %s, want LostPart error envelope", w.Body.String())
			}
		})
	}
}

func TestGetJobValidatesTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/svr/job", GetJob)

	for _, bad := range []string{"abc", "0", "-5"} {
		req := httptest.NewRequest(http.MethodGet, "/svr/job?taskId="+bad, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"code":500`) {
			t.Fatalf("taskId=%q status=%d body=%s, want validation error", bad, w.Code, w.Body.String())
		}
	}
}

func TestFlexibleIDParsesNumericAndStringJSONIDs(t *testing.T) {
	var req updateJobRequest
	if err := json.Unmarshal([]byte(`{"id":1000000,"taskId":"42","pause":true}`), &req); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if !req.ID.set || !req.ID.valid || req.ID.value != 1000000 {
		t.Fatalf("ID = %+v, want valid 1000000", req.ID)
	}
	if !req.TaskID.set || !req.TaskID.valid || req.TaskID.value != 42 {
		t.Fatalf("TaskID = %+v, want valid 42", req.TaskID)
	}

	var bad updateJobRequest
	if err := json.Unmarshal([]byte(`{"id":0,"taskId":1.5}`), &bad); err != nil {
		t.Fatalf("Unmarshal(bad) error: %v", err)
	}
	if !bad.ID.set || bad.ID.valid {
		t.Fatalf("ID = %+v, want set=true valid=false for 0", bad.ID)
	}
	if !bad.TaskID.set || bad.TaskID.valid {
		t.Fatalf("TaskID = %+v, want set=true valid=false for 1.5", bad.TaskID)
	}
}

func TestHasPresentJobIDIgnoresNullZeroAndEmpty(t *testing.T) {
	for _, absent := range []interface{}{nil, "", "   ", "0", 0, int64(0), float64(0), json.Number("0")} {
		if hasPresentJobID(absent) {
			t.Fatalf("hasPresentJobID(%#v) = true, want false", absent)
		}
	}
	for _, present := range []interface{}{1, int64(42), float64(7), "5", json.Number("9")} {
		if !hasPresentJobID(present) {
			t.Fatalf("hasPresentJobID(%#v) = false, want true", present)
		}
	}
}
