package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
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

type asResult struct {
	AS       string   `json:"as"`
	Name     string   `json:"name"`
	Prefixes []string `json:"prefixes"`
}

func main() {
	client := apiClient{
		baseURL: ripeStatURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
	logger := log.New(os.Stderr, "prefixes: ", 0)
	if err := run(context.Background(), os.Args[1:], os.Stdout, client, logger); err != nil {
		fmt.Fprintln(os.Stderr, "prefixes:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output io.Writer, client apiClient, logger *log.Logger) error {
	// Check every argument before making requests or writing output.
	var asns []string
	includeIPv6 := false
	quiet := false
	format := "minimal"
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "-6":
			includeIPv6 = true
		case "-q":
			quiet = true
		case "-o":
			if i+1 >= len(args) {
				return errors.New("-o requires a format: json, csv, or minimal")
			}
			i++
			format = args[i]
			if format != "json" && format != "csv" && format != "minimal" {
				return fmt.Errorf("invalid output format %q: expected json, csv, or minimal", format)
			}
		default:
			asn, err := parseASN(arg)
			if err != nil {
				return err
			}
			asns = append(asns, asn)
		}
	}
	if len(asns) == 0 {
		return errors.New("usage: prefixes [-6] [-q] [-o json|csv|minimal] AS15348 [AS32491 ...]")
	}
	if quiet {
		logger = log.New(io.Discard, "", 0)
	}

	results := make([]asResult, 0, len(asns))
	addressFamilies := "v4"
	if includeIPv6 {
		addressFamilies = "v4,v6"
	}
	var totalV4, totalV6 big.Int
	var totalPrefixes uint64
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

		prefixStrings := make([]string, len(prefixes))
		for i, prefix := range prefixes {
			prefixStrings[i] = prefix.String()
		}
		results = append(results, asResult{AS: asn, Name: name, Prefixes: prefixStrings})

		var asnV4, asnV6 big.Int
		for _, prefix := range prefixes {
			if prefix.Addr().Is4() {
				asnV4.Add(&asnV4, ipCount(prefix))
			} else {
				asnV6.Add(&asnV6, ipCount(prefix))
			}
		}
		totalV4.Add(&totalV4, &asnV4)
		totalV6.Add(&totalV6, &asnV6)
		totalPrefixes += uint64(len(prefixes))

		summary := fmt.Sprintf("called RIPE to fetch %s: got %d prefixes with total of %s IPv4 IPs", asn, len(prefixes), &asnV4)
		if includeIPv6 {
			summary += fmt.Sprintf(" and %s IPv6 IPs", &asnV6)
		}
		logger.Print(summary)
	}

	total := fmt.Sprintf("total: got %d prefixes with total of %s IPv4 IPs", totalPrefixes, &totalV4)
	if includeIPv6 {
		total += fmt.Sprintf(" and %s IPv6 IPs", &totalV6)
	}
	logger.Print(total)

	switch format {
	case "json":
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(results); err != nil {
			return fmt.Errorf("write JSON: %w", err)
		}
	case "csv":
		rows := [][]string{{"AS", "Name", "Prefix"}}
		for _, result := range results {
			if len(result.Prefixes) == 0 {
				rows = append(rows, []string{result.AS, result.Name, ""})
			}
			for _, prefix := range result.Prefixes {
				rows = append(rows, []string{result.AS, result.Name, prefix})
			}
		}
		w := csv.NewWriter(output)
		if err := w.WriteAll(rows); err != nil {
			return fmt.Errorf("write CSV: %w", err)
		}
	case "minimal":
		for _, result := range results {
			for _, prefix := range result.Prefixes {
				if _, err := fmt.Fprintln(output, prefix); err != nil {
					return fmt.Errorf("write output: %w", err)
				}
			}
		}
	}
	return nil
}

// ipCount returns the number of addresses contained in a prefix.
func ipCount(prefix netip.Prefix) *big.Int {
	bits := 128
	if prefix.Addr().Is4() {
		bits = 32
	}
	return new(big.Int).Lsh(big.NewInt(1), uint(bits-prefix.Bits()))
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
