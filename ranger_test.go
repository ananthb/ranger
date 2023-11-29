package ranger

import (
	"bytes"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDo(t *testing.T) {
	content := makeData(1024 * 10)
	server := makeHTTPServer(t, content)

	type expected struct {
		body               []byte
		contentLength      int64
		contentRangeHeader string
	}

	testCases := []struct {
		name        string
		rangeHeader string
		chunkSize   int64
		workers     int64
		expected    expected
		err         bool
	}{
		{
			name:        "Start at 42",
			rangeHeader: "bytes=42-",
			chunkSize:   1024,
			workers:     100,
			expected: expected{
				body:               content[42:],
				contentLength:      int64(len(content[42:])),
				contentRangeHeader: "bytes 42-10239/10240",
			},
		},
		{
			name:        "small range",
			rangeHeader: "bytes=42-83",
			chunkSize:   5,
			workers:     10,
			expected: expected{
				body:               content[42:84],
				contentLength:      int64(len(content[42:84])),
				contentRangeHeader: "bytes 42-83/10240",
			},
		},
		{
			name:        "error fetching multiple ranges",
			rangeHeader: "bytes=100-200,300-400",
			err:         true,
		},
		{
			name:      "1 byte chunk",
			chunkSize: 1,
			workers:   1,
			expected:  expected{body: content, contentLength: int64(len(content))},
		},
		{
			name:      "3KiB chunks",
			chunkSize: 3 * 1024,
			workers:   5,
			expected:  expected{body: content, contentLength: int64(len(content))},
		},
		{
			name:      "2KiB chunks",
			chunkSize: 2048,
			workers:   8,
			expected:  expected{body: content, contentLength: int64(len(content))},
		},
		{
			name:      "16MiB chunks",
			chunkSize: 16 * 1024 * 1024,
			workers:   100,
			expected:  expected{body: content, contentLength: int64(len(content))},
		},
		{
			name:      "single chunk buffer",
			chunkSize: 1024,
			workers:   1,
			expected:  expected{body: content, contentLength: int64(len(content))},
		},
		{name: "invalid range", rangeHeader: "bytes=100-50", err: true},
		{name: "0 chunk size", workers: 100, err: true},
		{name: "0 buffer number", chunkSize: 1024, err: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, server.URL, nil)
			assert.NoError(t, err)
			if testCase.rangeHeader != "" {
				req.Header.Set("Range", testCase.rangeHeader)
			}
			resp, err := Do(nil, req, testCase.chunkSize, testCase.workers)
			if testCase.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				defer resp.Body.Close()
				assert.Equal(t, resp.ContentLength, testCase.expected.contentLength)
				assert.Equal(t, resp.Header.Get(headerNameContentRange), testCase.expected.contentRangeHeader)
				assert.NoError(t, iotest.TestReader(resp.Body, testCase.expected.body))
			}
		})
	}
}

func TestNewClient(t *testing.T) {
	content := makeData(1024 * 10)
	server := makeHTTPServer(t, content)
	client := NewClient(nil, 1024, 100)
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	assert.NoError(t, err)
	resp, err := client.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.NoError(t, iotest.TestReader(resp.Body, content))
}

func TestNewRoundTripper(t *testing.T) {
	content := makeData(1024 * 10)
	server := makeHTTPServer(t, content)
	transport := NewRoundTripper(nil, 1024, 100)
	client := &http.Client{Transport: transport}
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	assert.NoError(t, err)
	resp, err := client.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.NoError(t, iotest.TestReader(resp.Body, content))
}

func makeData(size int) []byte {
	rnd := rand.New(rand.NewSource(42))
	content := make([]byte, size)
	rnd.Read(content)
	return content
}

func makeHTTPServer(t *testing.T, content []byte) *httptest.Server {
	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.ServeContent(writer, request, "", time.Now(), bytes.NewReader(content))
		}),
	)
	return server
}
