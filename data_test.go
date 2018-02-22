package imageproxy

import (
	"net/http"
	"net/url"
	"testing"
)

var emptyOptions = Options{}

func TestOptions_String(t *testing.T) {
	tests := []struct {
		Options Options
		String  string
	}{
		{
			emptyOptions,
			"0x0",
		},
		{
			Options{1, 2, true, 90, true, true, 80, "", false, "", 0, 0, 0, 0, false},
			"1x2,fit,r90,fv,fh,q80",
		},
		{
			Options{0.15, 1.3, false, 45, false, false, 95, "c0ffee", false, "png", 0, 0, 0, 0, false},
			"0.15x1.3,r45,q95,sc0ffee,png",
		},
		{
			Options{0.15, 1.3, false, 45, false, false, 95, "c0ffee", false, "", 100, 200, 0, 0, false},
			"0.15x1.3,r45,q95,sc0ffee,cx100,cy200",
		},
		{
			Options{0.15, 1.3, false, 45, false, false, 95, "c0ffee", false, "png", 100, 200, 300, 400, false},
			"0.15x1.3,r45,q95,sc0ffee,png,cx100,cy200,cw300,ch400",
		},
	}

	for i, tt := range tests {
		if got, want := tt.Options.String(), tt.String; got != want {
			t.Errorf("%d. Options.String returned %v, want %v", i, got, want)
		}
	}
}

func TestParseFormValues(t *testing.T) {
	tests := []struct {
		InputQS string
		Options Options
	}{
		{"", emptyOptions},
		{"x", emptyOptions},
		{"r", emptyOptions},
		{"0", emptyOptions},
		{"crop=,,,,", emptyOptions},

		// size variations
		{"width=1", Options{Width: 1}},
		{"height=1", Options{Height: 1}},
		{"width=1&height=2", Options{Width: 1, Height: 2}},
		{"width=-1&height=-2", Options{Width: -1, Height: -2}},
		{"width=0.1&height=0.2", Options{Width: 0.1, Height: 0.2}},
		{"size=1", Options{Width: 1, Height: 1}},
		{"size=0.1", Options{Width: 0.1, Height: 0.1}},

		// additional flags
		{"mode=fit", Options{Fit: true}},
		{"rotate=90", Options{Rotate: 90}},
		{"flip=v", Options{FlipVertical: true}},
		{"flip=h", Options{FlipHorizontal: true}},
		{"format=jpeg", Options{Format: "jpeg"}},

		// mix of valid and invalid flags
		{"FOO=BAR&size=1&BAR=foo&rotate=90&BAZ=DAS", Options{Width: 1, Height: 1, Rotate: 90}},

		// flags, in different orders
		{"quality=70&width=1&height=2&mode=fit&rotate=90&flip=v&flip=h&signature=c0ffee&format=png", Options{1, 2, true, 90, true, true, 70, "c0ffee", false, "png", 0, 0, 0, 0, false}},
		{"rotate=90&flip=h&signature=c0ffee&format=png&quality=90&width=1&height=2&flip=v&mode=fit", Options{1, 2, true, 90, true, true, 90, "c0ffee", false, "png", 0, 0, 0, 0, false}},

		// all flags, in different orders with crop
		{"quality=70&width=1&height=2&mode=fit&crop=100,200,300,400&rotate=90&flip=v&flip=h&signature=c0ffee&format=png", Options{1, 2, true, 90, true, true, 70, "c0ffee", false, "png", 100, 200, 300, 400, false}},
		{"rotate=90&flip=h&signature=c0ffee&format=png&crop=100,200,300,400&quality=90&width=1&height=2&flip=v&mode=fit", Options{1, 2, true, 90, true, true, 90, "c0ffee", false, "png", 100, 200, 300, 400, false}},

		// all flags, in different orders with crop & different resizes
		{"quality=70&crop=100,200,300,400&height=2&mode=fit&rotate=90&flip=v&flip=h&signature=c0ffee&format=png", Options{0, 2, true, 90, true, true, 70, "c0ffee", false, "png", 100, 200, 300, 400, false}},
		{"crop=100,200,300,400&rotate=90&flip=h&quality=90&signature=c0ffee&format=png&width=1&flip=v&mode=fit", Options{1, 0, true, 90, true, true, 90, "c0ffee", false, "png", 100, 200, 300, 400, false}},
		{"crop=100,200,300,400&rotate=90&flip=h&signature=c0ffee&flip=v&format=png&quality=90&mode=fit", Options{0, 0, true, 90, true, true, 90, "c0ffee", false, "png", 100, 200, 300, 400, false}},
		{"crop=100,200,0,400&rotate=90&quality=90&flip=h&signature=c0ffee&format=png&flip=v&mode=fit&width=123&height=321", Options{123, 321, true, 90, true, true, 90, "c0ffee", false, "png", 100, 200, 0, 400, false}},
		{"flip=v&width=123&height=321&crop=100,200,300,400&quality=90&rotate=90&flip=h&signature=c0ffee&format=png&mode=fit", Options{123, 321, true, 90, true, true, 90, "c0ffee", false, "png", 100, 200, 300, 400, false}},
	}

	for _, tt := range tests {
		input, err := url.ParseQuery(tt.InputQS)
		if err != nil {
			panic(err)
		}

		if got, want := ParseFormValues(input), tt.Options; got != want {
			t.Errorf("ParseFormValues(%q) returned %#v, want %#v", tt.InputQS, got, want)
		}
	}
}

// Test that request URLs are properly parsed into Options and RemoteURL.  This
// test verifies that invalid remote URLs throw errors, and that valid
// combinations of Options and URL are accept.  This does not exhaustively test
// the various Options that can be specified; see TestParseOptions for that.
func TestNewRequest(t *testing.T) {
	tests := []struct {
		URL         string  // input URL to parse as an imageproxy request
		RemoteURL   string  // expected URL of remote image parsed from input
		Options     Options // expected options parsed from input
		ExpectError bool    // whether an error is expected from NewRequest
	}{
		// invalid URLs
		{"http://localhost/", "", emptyOptions, true},
		{"http://localhost/1/", "", emptyOptions, true},
		{"http://localhost//example.com/foo", "", emptyOptions, true},
		{"http://localhost//ftp://example.com/foo", "", emptyOptions, true},

		// invalid options.  These won't return errors, but will not fully parse the options
		{
			"http://localhost/s/http://example.com/",
			"http://example.com/", emptyOptions, false,
		},
		{
			"http://localhost/1xs/http://example.com/",
			"http://example.com/", Options{Width: 1}, false,
		},

		// valid URLs
		{
			"http://localhost/http://example.com/foo",
			"http://example.com/foo", emptyOptions, false,
		},
		{
			"http://localhost//http://example.com/foo",
			"http://example.com/foo", emptyOptions, false,
		},
		{
			"http://localhost//https://example.com/foo",
			"https://example.com/foo", emptyOptions, false,
		},
		{
			"http://localhost/1x2/http://example.com/foo",
			"http://example.com/foo", Options{Width: 1, Height: 2}, false,
		},
		{
			"http://localhost//http://example.com/foo?bar",
			"http://example.com/foo?bar", emptyOptions, false,
		},
		{
			"http://localhost/http:/example.com/foo",
			"http://example.com/foo", emptyOptions, false,
		},
		{
			"http://localhost/http:///example.com/foo",
			"http://example.com/foo", emptyOptions, false,
		},
		{ // escaped path
			"http://localhost/http://example.com/%2C",
			"http://example.com/%2C", emptyOptions, false,
		},

		// valid URLs with the prefix
		{
			"http://localhost/prefix/http://example.com/foo",
			"http://example.com/foo", emptyOptions, false,
		},
		{
			"http://localhost/prefix//http://example.com/foo",
			"http://example.com/foo", emptyOptions, false,
		},
		{
			"http://localhost/prefix//https://example.com/foo",
			"https://example.com/foo", emptyOptions, false,
		},
		{
			"http://localhost/prefix/1x2/http://example.com/foo",
			"http://example.com/foo", Options{Width: 1, Height: 2}, false,
		},
		{
			"http://localhost/prefix//http://example.com/foo?bar",
			"http://example.com/foo?bar", emptyOptions, false,
		},
		{
			"http://localhost/prefix/http:/example.com/foo",
			"http://example.com/foo", emptyOptions, false,
		},
		{
			"http://localhost/prefix/http:///example.com/foo",
			"http://example.com/foo", emptyOptions, false,
		},
		{ // escaped path
			"http://localhost/prefix/http://example.com/%2C",
			"http://example.com/%2C", emptyOptions, false,
		},
	}

	// Try with both versions of the same prefix. The results should be same.
	prefixes := []string{"/prefix/", "/prefix"}

	for _, prefix := range prefixes {
		for _, tt := range tests {
			req, err := http.NewRequest("GET", tt.URL, nil)
			if err != nil {
				t.Errorf("http.NewRequest(%q) returned error: %v", tt.URL, err)
				continue
			}

			r, err := NewRequest(req, nil, prefix)
			if tt.ExpectError {
				if err == nil {
					t.Errorf("NewRequest(%v) did not return expected error", req)
				}
				continue
			} else if err != nil {
				t.Errorf("NewRequest(%v) return unexpected error: %v", req, err)
				continue
			}

			if got, want := r.URL.String(), tt.RemoteURL; got != want {
				t.Errorf("NewRequest(%q) request URL = %v, want %v", tt.URL, got, want)
			}
			if got, want := r.Options, tt.Options; got != want {
				t.Errorf("NewRequest(%q) request options = %v, want %v", tt.URL, got, want)
			}
		}
	}
}
