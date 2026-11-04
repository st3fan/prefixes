package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

const ripeStatURL = "https://stat.ripe.net"

type apiClient struct {
	baseURL string
	http    *http.Client
}

type apiResponse struct {
	Status     string          `json:"status"`
	StatusCode int             `json:"status_code"`
	Data       json.RawMessage `json:"data"`
}

type overviewData struct {
	Holder *string `json:"holder"`
}

type prefixesData struct {
	Prefixes struct {
		V4 struct {
			Originating []string `json:"originating"`
		} `json:"v4"`
		V6 struct {
			Originating []string `json:"originating"`
		} `json:"v6"`
	} `json:"prefixes"`
}

func main() {
	client := apiClient{
		baseURL: ripeStatURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
	if err := run(context.Background(), os.Args[1:], os.Stdout, client); err != nil {
		fmt.Fprintln(os.Stderr, "prefixes:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output io.Writer, client apiClient) error {
	// Check every argument before making requests or writing output.
	var asns []string
	includeIPv6 := false
	for _, arg := range args {
		if arg == "-6" {
			includeIPv6 = true
			continue
		}
		asn, err := parseASN(arg)
		if err != nil {
			return err
		}
		asns = append(asns, asn)
	}
	if len(asns) == 0 {
		return errors.New("usage: prefixes [-6] AS15348 [AS32491 ...]")
	}

	rows := [][]string{{"AS", "Name", "Prefix"}}
	addressFamilies := "v4"
	if includeIPv6 {
		addressFamilies = "v4,v6"
	}
	for _, asn := range asns {
		var overview overviewData
		if err := client.fetch(ctx, "as-overview", asn, nil, &overview); err != nil {
			return fmt.Errorf("%s name: %w", asn, err)
		}
		name := ""
		if overview.Holder != nil {
			name = *overview.Holder
		}

		var data prefixesData
		if err := client.fetch(ctx, "ris-prefixes", asn, url.Values{
			"af":            {addressFamilies},
			"list_prefixes": {"true"},
			"types":         {"o"},
		}, &data); err != nil {
			return fmt.Errorf("%s prefixes: %w", asn, err)
		}

		seen := make(map[netip.Prefix]struct{})
		var prefixes []netip.Prefix
		values := data.Prefixes.V4.Originating
		if includeIPv6 {
			values = append(values, data.Prefixes.V6.Originating...)
		}
		for _, value := range values {
			prefix, err := netip.ParsePrefix(value)
			if err != nil {
				return fmt.Errorf("%s: invalid prefix %q: %w", asn, value, err)
			}
			if _, ok := seen[prefix]; !ok {
				seen[prefix] = struct{}{}
				prefixes = append(prefixes, prefix)
			}
		}
		slices.SortFunc(prefixes, func(a, b netip.Prefix) int {
			if n := a.Addr().Compare(b.Addr()); n != 0 {
				return n
			}
			return a.Bits() - b.Bits()
		})

		if len(prefixes) == 0 {
			rows = append(rows, []string{asn, name, ""})
		}
		for _, prefix := range prefixes {
			rows = append(rows, []string{asn, name, prefix.String()})
		}
	}

	w := csv.NewWriter(output)
	if err := w.WriteAll(rows); err != nil {
		return fmt.Errorf("write CSV: %w", err)
	}
	return nil
}

func parseASN(value string) (string, error) {
	if len(value) < 3 || !strings.EqualFold(value[:2], "AS") {
		return "", fmt.Errorf("invalid AS number %q: expected AS followed by a number", value)
	}
	n, err := strconv.ParseUint(value[2:], 10, 32)
	if err != nil || n == 0 {
		return "", fmt.Errorf("invalid AS number %q: expected AS1 through AS4294967295", value)
	}
	return "AS" + strconv.FormatUint(n, 10), nil
}

func (c apiClient) fetch(ctx context.Context, endpoint, asn string, params url.Values, target any) error {
	if params == nil {
		params = make(url.Values)
	}
	params.Set("resource", asn)
	requestURL := strings.TrimRight(c.baseURL, "/") + "/data/" + endpoint + "/data.json?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("RIPEstat returned HTTP %s", resp.Status)
	}
	var result apiResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&result); err != nil {
		return fmt.Errorf("decode RIPEstat response: %w", err)
	}
	if result.Status != "ok" || result.StatusCode != http.StatusOK {
		return fmt.Errorf("RIPEstat returned status %q (%d)", result.Status, result.StatusCode)
	}
	if err := json.Unmarshal(result.Data, target); err != nil {
		return fmt.Errorf("decode RIPEstat data: %w", err)
	}
	return nil
}
