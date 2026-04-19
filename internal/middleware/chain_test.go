package middleware

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestChainOrder(t *testing.T) {
	sequence := make([]string, 0, 5)
	m1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sequence = append(sequence, "m1-before")
			next.ServeHTTP(w, r)
			sequence = append(sequence, "m1-after")
		})
	}
	m2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sequence = append(sequence, "m2-before")
			next.ServeHTTP(w, r)
			sequence = append(sequence, "m2-after")
		})
	}
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sequence = append(sequence, "final")
		w.WriteHeader(http.StatusNoContent)
	})

	h := Chain(final, m1, m2)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rr, req)

	want := []string{"m1-before", "m2-before", "final", "m2-after", "m1-after"}
	if !reflect.DeepEqual(sequence, want) {
		t.Fatalf("sequence = %#v, want %#v", sequence, want)
	}
}

func TestChainIgnoresNilMiddleware(t *testing.T) {
	called := false
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	h := Chain(final, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rr, req)

	if !called {
		t.Fatal("expected final handler to be called")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}
