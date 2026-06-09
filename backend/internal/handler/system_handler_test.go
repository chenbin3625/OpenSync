package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"opensync/internal/config"
	"opensync/internal/mapper"
	"opensync/internal/middleware"
	"opensync/internal/model"
	"opensync/pkg/crypto"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func TestLoginFailuresDoNotLockOutDifferentClientScope(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if _, err := db.Exec(`CREATE TABLE user_list(
		id integer primary key autoincrement,
		userName text,
		passwd text,
		sqlVersion integer,
		createTime integer DEFAULT 1
	)`); err != nil {
		t.Fatalf("create user_list: %v", err)
	}
	hash, err := crypto.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	if _, err := db.Exec("INSERT INTO user_list(id, userName, passwd) VALUES (1, 'admin', ?)", hash); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	restoreDB := mapper.SetDBForTest(db)
	t.Cleanup(restoreDB)
	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		Server: config.ServerConfig{
			Expires:   7,
			PasswdStr: "test-login-secret",
		},
	})
	t.Cleanup(func() {
		config.SetConfigForTest(oldConfig)
	})
	middleware.InitSecureCookie()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		c.JSON(http.StatusInternalServerError, model.Error(fmt.Sprintf("%v", recovered)))
	}))
	router.POST("/svr/noAuth/login", Login)

	for i := 0; i < 4; i++ {
		w := performLogin(router, "192.0.2.10:1234", "admin", "bad-password")
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("bad login %d status = %d, want recovery status %d", i+1, w.Code, http.StatusInternalServerError)
		}
	}

	w := performLogin(router, "192.0.2.11:1234", "admin", "correct-password")
	if w.Code != http.StatusOK {
		t.Fatalf("valid login from another client status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"code":200`) {
		t.Fatalf("valid login response = %s, want success code", w.Body.String())
	}
}

func performLogin(router http.Handler, remoteAddr, userName, passwd string) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"userName":%q,"passwd":%q}`, userName, passwd)
	req := httptest.NewRequest(http.MethodPost, "/svr/noAuth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
