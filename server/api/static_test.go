package api

import (
	"cloudease-lite/server/config"
	live "cloudease-lite/server/websocket"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticAssetsNeverFallBackToHTML(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"index.html": "<!doctype html><div id=app></div>", "assets/app.js": "console.log('ready')", "assets/app.css": "body{color:black}"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	router := New(nil, config.Config{WebDir: dir}, live.New()).Router()
	for _, test := range []struct {
		path    string
		code    int
		content string
	}{
		{"/assets/app.js", 200, "javascript"}, {"/assets/app.css", 200, "text/css"},
		{"/assets/missing.js", 404, "application/json"}, {"/assets/missing.css", 404, "application/json"},
		{"/login", 200, "text/html"},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("GET", test.path, nil))
			if response.Code != test.code || !strings.Contains(response.Header().Get("Content-Type"), test.content) {
				t.Fatalf("status=%d content-type=%s body=%s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
			}
		})
	}
}
