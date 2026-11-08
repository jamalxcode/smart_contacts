package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testZip(t *testing.T) []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, _ := zw.Create("a.txt")
	w.Write(bytes.Repeat([]byte("smart contacts "), 20000))
	zw.Close()
	return b.Bytes()
}

func fastRetries(t *testing.T, urls ...string) {
	ow, ou, oh, os_ := retryWaits, downloadURLs, headerWait, stallWait
	retryWaits = []time.Duration{0, time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond}
	downloadURLs = func() []string { return urls }
	headerWait, stallWait = 300*time.Millisecond, 300*time.Millisecond
	t.Cleanup(func() { retryWaits, downloadURLs, headerWait, stallWait = ow, ou, oh, os_ })
}

// 504 twice, then a connection that drops halfway, then the rest via Range.
func TestDownloadRetriesAndResumes(t *testing.T) {
	data := testZip(t)
	var calls int32
	var resumed int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		switch {
		case n <= 2:
			http.Error(w, "busy", http.StatusGatewayTimeout)
		case n == 3: // promise everything, send half, hang up
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.WriteHeader(200)
			w.Write(data[:len(data)/2])
			w.(http.Flusher).Flush()
			if hj, ok := w.(http.Hijacker); ok {
				c, _, _ := hj.Hijack()
				c.Close()
			}
		default:
			rg := r.Header.Get("Range")
			var from int
			fmt.Sscanf(rg, "bytes=%d-", &from)
			if from > 0 {
				atomic.AddInt32(&resumed, 1)
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(data)-from))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(data[from:])
		}
	}))
	defer srv.Close()
	fastRetries(t, srv.URL)
	var msgs []string
	path := filepath.Join(t.TempDir(), "u.zip")
	if err := downloadZip(context.Background(), path, nil, func(s string) { msgs = append(msgs, s) }); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, data) {
		t.Fatalf("file differs: %d vs %d bytes", len(got), len(data))
	}
	if resumed != 1 {
		t.Fatalf("expected a resumed download, got %d", resumed)
	}
	if len(msgs) == 0 || !strings.Contains(msgs[0], "trying again") {
		t.Fatalf("no retry message: %v", msgs)
	}
}

// The first route always 504s; the second (API asset link) works.
func TestDownloadFallsBack(t *testing.T) {
	data := testZip(t)
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "busy", http.StatusGatewayTimeout)
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer good.Close()
	fastRetries(t, bad.URL, good.URL)
	path := filepath.Join(t.TempDir(), "u.zip")
	if err := downloadZip(context.Background(), path, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, data) {
		t.Fatal("file differs")
	}
}

// When nothing works the message says what to do.
func TestDownloadGivesUpKindly(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "busy", http.StatusGatewayTimeout)
	}))
	defer bad.Close()
	fastRetries(t, bad.URL)
	err := downloadZip(context.Background(), filepath.Join(t.TempDir(), "u.zip"), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "install.sh") || !strings.Contains(err.Error(), "few minutes") {
		t.Fatalf("unhelpful error: %v", err)
	}
}

// A server that never answers, and one that goes quiet mid-download, don't hang the update:
// the attempt is dropped and the other link is used.
func TestDownloadDoesNotHang(t *testing.T) {
	data := testZip(t)
	silent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer silent.Close()
	stalls := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(200)
		w.Write(data[:len(data)/2])
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer stalls.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "u.zip", time.Time{}, bytes.NewReader(data))
	}))
	defer good.Close()
	for _, bad := range []string{silent.URL, stalls.URL} {
		fastRetries(t, bad, good.URL)
		start := time.Now()
		path := filepath.Join(t.TempDir(), "u.zip")
		if err := downloadZip(context.Background(), path, nil, nil); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(path); !bytes.Equal(got, data) {
			t.Fatal("file differs")
		}
		if d := time.Since(start); d > 3*time.Second {
			t.Fatalf("took %v", d)
		}
	}
}

// Cancel stops at once.
func TestDownloadCancel(t *testing.T) {
	silent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer silent.Close()
	fastRetries(t, silent.URL)
	headerWait = 10 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	start := time.Now()
	err := downloadZip(ctx, filepath.Join(t.TempDir(), "u.zip"), nil, nil)
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("cancel didn't stop it: %v after %v", err, time.Since(start))
	}
}
