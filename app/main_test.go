package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyHandler_WithAllowedTime(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "Hello, world"}`))
	}))
	defer ts.Close()

	req, err := http.NewRequest("GET", fmt.Sprintf("/?endpoint=%s&crontab=* * * * *", ts.URL), http.NoBody)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(proxyHandler(100))

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, `{"message": "Hello, world"}`, rr.Body.String())
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
}

func TestProxyHandler_WithNotAllowedTime(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "Hello, world"}`))
	}))
	defer ts.Close()

	req, err := http.NewRequest("GET", fmt.Sprintf("/?endpoint=%s&crontab=0 0 31 2 *", ts.URL), http.NoBody)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(proxyHandler(100))

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)

	expected := "No cached response available\n"
	assert.Equal(t, expected, rr.Body.String())
}

func TestProxyHandler_WithNotAllowedTimeUnderscored(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "Hello, world"}`))
	}))
	defer ts.Close()

	req, err := http.NewRequest("GET", fmt.Sprintf("/?endpoint=%s&crontab=0_0_31_2_*", ts.URL), http.NoBody)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(proxyHandler(100))

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)

	expected := "No cached response available\n"
	assert.Equal(t, expected, rr.Body.String())
}

func TestProxyHandler_WithNInvalidCron(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "Hello, world"}`))
	}))
	defer ts.Close()

	req, err := http.NewRequest("GET", fmt.Sprintf("/?endpoint=%s&crontab=blah", ts.URL), http.NoBody)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(proxyHandler(100))

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "failed to parse crontab")
}

func TestProxyHandler_WithNotAllowedTimeCached(t *testing.T) {
	// simulate a request within the allowed time to cache the response
	reqCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reqCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "Hello, world"}`))
	}))
	defer ts.Close()

	reqAllowed, err := http.NewRequest("GET", fmt.Sprintf("/?endpoint=%s&crontab=* * * * *", ts.URL), http.NoBody)
	assert.NoError(t, err)

	rrAllowed := httptest.NewRecorder()
	handler := http.HandlerFunc(proxyHandler(100))

	handler.ServeHTTP(rrAllowed, reqAllowed)
	assert.Equal(t, http.StatusOK, rrAllowed.Code)
	assert.Equal(t, `{"message": "Hello, world"}`, rrAllowed.Body.String())

	// simulate a request outside the allowed time and check for cached response
	reqNotAllowed, err := http.NewRequest("GET", fmt.Sprintf("/?endpoint=%s&crontab=0 0 31 2 *", ts.URL), http.NoBody)
	assert.NoError(t, err)

	rrNotAllowed := httptest.NewRecorder()

	handler.ServeHTTP(rrNotAllowed, reqNotAllowed)
	assert.Equal(t, http.StatusOK, rrNotAllowed.Code)
	assert.Equal(t, `{"message": "Hello, world"}`, rrNotAllowed.Body.String())
	assert.Equal(t, "application/json", rrNotAllowed.Header().Get("Content-Type"))

	// check if the request was made only once
	assert.Equal(t, 1, reqCount)
}

func TestIsAllowedTime_WithValidCrontab(t *testing.T) {
	crontab := "* * * * *"
	ok, err := isAllowedTime(crontab)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestIsAllowedTime_WithInvalidCrontab(t *testing.T) {
	crontab := "invalid crontab"
	_, err := isAllowedTime(crontab)
	require.Error(t, err)
}

func TestCopyHeaders(t *testing.T) {
	src := http.Header{}
	src.Add("Content-Type", "application/json")
	src.Add("Authorization", "Bearer token")

	dst := http.Header{}

	copyHeaders(dst, src)

	assert.Equal(t, "application/json", dst.Get("Content-Type"))
	assert.Equal(t, "Bearer token", dst.Get("Authorization"))
}

func TestRun_WithServerShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	setupLog(true, false)
	err := run(ctx, options{Port: 9999, Dbg: true})
	assert.NoError(t, err)
}
