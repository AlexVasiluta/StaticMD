package jsrunner

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func gencgi(t *testing.T, data string) *CGI {
	cgi, err := NewCGI(strings.NewReader(data))
	if err != nil {
		t.Fatalf("expected error to be nil got %v", err)
	}
	return cgi
}

func TestSimple(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, `/cf?name=12345`, nil)
	w := httptest.NewRecorder()

	c := gencgi(t, `
function handle(data) {
	return {errCode: 200, status: "success", data: "ana are mere"} // retData("error", "error", {"cf": "nu bine"})
}
`)
	if err := c.Execute(w, req); err != nil {
		t.Fatalf("expected error to be nil got %v", err)
	}

	res := w.Result()
	defer res.Body.Close()
	val, _ := io.ReadAll(res.Body)
	t.Log(string(val))
	t.Log(res.Status)
}
