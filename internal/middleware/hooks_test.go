package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHooksInvokesBeforeAndAfterCallbacks(t *testing.T) {
	beforeCalled := 0
	afterCalled := 0
	var gotStatus int
	var gotLatency time.Duration

	hooks := HookSet{
		BeforeRequest: []func(*http.Request){
			func(r *http.Request) {
				beforeCalled++
				r.Header.Set("X-Hook-Before", "1")
			},
		},
		AfterResponse: []func(*http.Request, int, time.Duration){
			func(r *http.Request, status int, latency time.Duration) {
				afterCalled++
				gotStatus = status
				gotLatency = latency
			},
		},
	}

	h := Hooks(hooks)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Hook-Before") != "1" {
			t.Fatal("expected before hook mutation")
		}
		w.WriteHeader(http.StatusAccepted)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hooks", nil)
	h.ServeHTTP(rr, req)

	if beforeCalled != 1 {
		t.Fatalf("beforeCalled = %d, want 1", beforeCalled)
	}
	if afterCalled != 1 {
		t.Fatalf("afterCalled = %d, want 1", afterCalled)
	}
	if gotStatus != http.StatusAccepted {
		t.Fatalf("after status = %d, want 202", gotStatus)
	}
	if gotLatency < 0 {
		t.Fatalf("latency = %s, want >= 0", gotLatency)
	}
}
