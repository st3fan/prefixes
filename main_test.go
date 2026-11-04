package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunCSV(t *testing.T) {
	for _, test := range []struct {
		name   string
		args   []string
		af     string
		wantV6 bool
	}{
		{name: "IPv4 default", args: []string{"-o", "csv", "as015348"}, af: "v4"},
		{name: "IPv6 flag", args: []string{"-o", "csv", "-6", "as015348"}, af: "v4,v6", wantV6: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("resource") != "AS15348" {
					t.Errorf("unexpected resource: %s", r.URL.Query().Get("resource"))
				}
				switch r.URL.Path {
				case "/data/as-overview/data.json":
					fmt.Fprint(w, `{"status":"ok","status_code":200,"data":{"holder":"Example, Inc."}}`)
				case "/data/ris-prefixes/data.json":
					query := r.URL.Query()
					if query.Get("list_prefixes") != "true" || query.Get("types") != "o" || query.Get("af") != test.af {
						t.Errorf("unexpected query: %s", r.URL.RawQuery)
					}
					fmt.Fprint(w, `{"status":"ok","status_code":200,"data":{"prefixes":{"v4":{"originating":["203.0.113.0/24","192.0.2.0/24","203.0.113.0/24"]},"v6":{"originating":["2001:db8::/32"]}}}}`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			var output bytes.Buffer
			if err := run(context.Background(), test.args, &output, apiClient{server.URL, server.Client()}, log.New(io.Discard, "", 0)); err != nil {
				t.Fatal(err)
			}
			want := "AS,Name,Prefix\nAS15348,\"Example, Inc.\",192.0.2.0/24\nAS15348,\"Example, Inc.\",203.0.113.0/24\n"
			if test.wantV6 {
				want += "AS15348,\"Example, Inc.\",2001:db8::/32\n"
			}
			if got := output.String(); got != want {
				t.Errorf("CSV output:\n got %q\nwant %q", got, want)
			}
		})
	}
}

func TestRunNoPrefixes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "as-overview") {
			fmt.Fprint(w, `{"status":"ok","status_code":200,"data":{"holder":null}}`)
			return
		}
		fmt.Fprint(w, `{"status":"ok","status_code":200,"data":{"prefixes":{}}}`)
	}))
	defer server.Close()

	var output bytes.Buffer
	if err := run(context.Background(), []string{"-o", "csv", "AS32491"}, &output, apiClient{server.URL, server.Client()}, log.New(io.Discard, "", 0)); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "AS,Name,Prefix\nAS32491,,\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRunRejectsInvalidInputWithoutOutput(t *testing.T) {
	var output bytes.Buffer
	err := run(context.Background(), []string{"AS15348", "ASnope"}, &output, apiClient{}, log.New(io.Discard, "", 0))
	if err == nil || !strings.Contains(err.Error(), "ASnope") {
		t.Fatalf("expected invalid AS error, got %v", err)
	}
	if output.Len() != 0 {
		t.Errorf("unexpected output: %q", output.String())
	}
}

func TestRunAvoidsPartialCSVOnAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	var output bytes.Buffer
	err := run(context.Background(), []string{"-o", "csv", "AS15348"}, &output, apiClient{server.URL, server.Client()}, log.New(io.Discard, "", 0))
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("expected HTTP error, got %v", err)
	}
	if output.Len() != 0 {
		t.Errorf("unexpected output: %q", output.String())
	}
}

func TestRunLogging(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/as-overview/data.json":
			fmt.Fprint(w, `{"status":"ok","status_code":200,"data":{"holder":"Example, Inc."}}`)
		case "/data/ris-prefixes/data.json":
			fmt.Fprint(w, `{"status":"ok","status_code":200,"data":{"prefixes":{"v4":{"originating":["203.0.113.0/24","192.0.2.0/24"]},"v6":{"originating":["2001:db8::/32"]}}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "IPv4 default",
			args: []string{"as015348"},
			want: "called RIPE to fetch AS15348: got 2 prefixes with total of 512 IPv4 IPs\n" +
				"total: got 2 prefixes with total of 512 IPv4 IPs\n",
		},
		{
			name: "IPv6 flag",
			args: []string{"-6", "as015348"},
			want: "called RIPE to fetch AS15348: got 3 prefixes with total of 512 IPv4 IPs and 79228162514264337593543950336 IPv6 IPs\n" +
				"total: got 3 prefixes with total of 512 IPv4 IPs and 79228162514264337593543950336 IPv6 IPs\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			var output bytes.Buffer
			if err := run(context.Background(), test.args, &output, apiClient{server.URL, server.Client()}, log.New(&logs, "", 0)); err != nil {
				t.Fatal(err)
			}
			if got := logs.String(); got != test.want {
				t.Errorf("log output:\n got %q\nwant %q", got, test.want)
			}
		})
	}
}

func TestRunQuietSuppressesLogging(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "as-overview") {
			fmt.Fprint(w, `{"status":"ok","status_code":200,"data":{"holder":"Example, Inc."}}`)
			return
		}
		fmt.Fprint(w, `{"status":"ok","status_code":200,"data":{"prefixes":{"v4":{"originating":["192.0.2.0/24"]}}}}`)
	}))
	defer server.Close()

	var logs bytes.Buffer
	var output bytes.Buffer
	if err := run(context.Background(), []string{"-q", "-o", "csv", "AS15348"}, &output, apiClient{server.URL, server.Client()}, log.New(&logs, "", 0)); err != nil {
		t.Fatal(err)
	}
	if logs.Len() != 0 {
		t.Errorf("expected no log output, got %q", logs.String())
	}
	if got, want := output.String(), "AS,Name,Prefix\nAS15348,\"Example, Inc.\",192.0.2.0/24\n"; got != want {
		t.Errorf("CSV output:\n got %q\nwant %q", got, want)
	}
}

func TestRunMinimalOutput(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "default is minimal",
			args: []string{"as015348"},
			want: "192.0.2.0/24\n203.0.113.0/24\n",
		},
		{
			name: "explicit minimal with IPv6",
			args: []string{"-o", "minimal", "-6", "as015348"},
			want: "192.0.2.0/24\n203.0.113.0/24\n2001:db8::/32\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "as-overview") {
					fmt.Fprint(w, `{"status":"ok","status_code":200,"data":{"holder":"Example, Inc."}}`)
					return
				}
				fmt.Fprint(w, `{"status":"ok","status_code":200,"data":{"prefixes":{"v4":{"originating":["203.0.113.0/24","192.0.2.0/24","203.0.113.0/24"]},"v6":{"originating":["2001:db8::/32"]}}}}`)
			}))
			defer server.Close()

			var output bytes.Buffer
			if err := run(context.Background(), test.args, &output, apiClient{server.URL, server.Client()}, log.New(io.Discard, "", 0)); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); got != test.want {
				t.Errorf("minimal output:\n got %q\nwant %q", got, test.want)
			}
		})
	}
}

func TestRunJSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "as-overview") {
			fmt.Fprint(w, `{"status":"ok","status_code":200,"data":{"holder":"Example, Inc."}}`)
			return
		}
		fmt.Fprint(w, `{"status":"ok","status_code":200,"data":{"prefixes":{"v4":{"originating":["203.0.113.0/24","192.0.2.0/24","203.0.113.0/24"]}}}}`)
	}))
	defer server.Close()

	var output bytes.Buffer
	if err := run(context.Background(), []string{"-o", "json", "as015348"}, &output, apiClient{server.URL, server.Client()}, log.New(io.Discard, "", 0)); err != nil {
		t.Fatal(err)
	}
	want := "[\n" +
		"  {\n" +
		"    \"as\": \"AS15348\",\n" +
		"    \"name\": \"Example, Inc.\",\n" +
		"    \"prefixes\": [\n" +
		"      \"192.0.2.0/24\",\n" +
		"      \"203.0.113.0/24\"\n" +
		"    ]\n" +
		"  }\n" +
		"]\n"
	if got := output.String(); got != want {
		t.Errorf("JSON output:\n got %q\nwant %q", got, want)
	}
}

func TestRunJSONOutputEmptyPrefixes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "as-overview") {
			fmt.Fprint(w, `{"status":"ok","status_code":200,"data":{"holder":null}}`)
			return
		}
		fmt.Fprint(w, `{"status":"ok","status_code":200,"data":{"prefixes":{}}}`)
	}))
	defer server.Close()

	var output bytes.Buffer
	if err := run(context.Background(), []string{"-o", "json", "AS32491"}, &output, apiClient{server.URL, server.Client()}, log.New(io.Discard, "", 0)); err != nil {
		t.Fatal(err)
	}
	want := "[\n  {\n    \"as\": \"AS32491\",\n    \"name\": \"\",\n    \"prefixes\": []\n  }\n]\n"
	if got := output.String(); got != want {
		t.Errorf("JSON output:\n got %q\nwant %q", got, want)
	}
}

func TestRunRejectsInvalidOutputFormat(t *testing.T) {
	for _, test := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "unknown format", args: []string{"-o", "yaml", "AS15348"}, wantErr: "invalid output format"},
		{name: "missing value", args: []string{"-o"}, wantErr: "requires a format"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := run(context.Background(), test.args, &output, apiClient{}, log.New(io.Discard, "", 0))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected %q error, got %v", test.wantErr, err)
			}
			if output.Len() != 0 {
				t.Errorf("unexpected output: %q", output.String())
			}
		})
	}
}
