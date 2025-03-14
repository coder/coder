package cliutil
import (
	"fmt"
	"errors"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"time"
)
const AWSIPRangesURL = "https://ip-ranges.amazonaws.com/ip-ranges.json"
type awsIPv4Prefix struct {
	Prefix             string `json:"ip_prefix"`
	Region             string `json:"region"`
	Service            string `json:"service"`
	NetworkBorderGroup string `json:"network_border_group"`
}
type awsIPv6Prefix struct {
	Prefix             string `json:"ipv6_prefix"`
	Region             string `json:"region"`
	Service            string `json:"service"`
	NetworkBorderGroup string `json:"network_border_group"`
}
type AWSIPRanges struct {
	V4 []netip.Prefix
	V6 []netip.Prefix
}
type awsIPRangesResponse struct {
	SyncToken    string          `json:"syncToken"`
	CreateDate   string          `json:"createDate"`
	IPV4Prefixes []awsIPv4Prefix `json:"prefixes"`
	IPV6Prefixes []awsIPv6Prefix `json:"ipv6_prefixes"`
}
func FetchAWSIPRanges(ctx context.Context, url string) (*AWSIPRanges, error) {
	client := &http.Client{}
	reqCtx, reqCancel := context.WithTimeout(ctx, 5*time.Second)
	defer reqCancel()
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, b)
	}
	var body awsIPRangesResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	if err != nil {
		return nil, fmt.Errorf("json decode: %w", err)
	}
	out := &AWSIPRanges{
		V4: make([]netip.Prefix, 0, len(body.IPV4Prefixes)),
		V6: make([]netip.Prefix, 0, len(body.IPV6Prefixes)),
	}
	for _, p := range body.IPV4Prefixes {
		prefix, err := netip.ParsePrefix(p.Prefix)
		if err != nil {
			return nil, fmt.Errorf("parse ip prefix: %w", err)
		}
		if prefix.Addr().Is6() {
			return nil, fmt.Errorf("ipv4 prefix contains ipv6 address: %s", p.Prefix)
		}
		out.V4 = append(out.V4, prefix)
	}
	for _, p := range body.IPV6Prefixes {
		prefix, err := netip.ParsePrefix(p.Prefix)
		if err != nil {
			return nil, fmt.Errorf("parse ip prefix: %w", err)
		}
		if prefix.Addr().Is4() {
			return nil, fmt.Errorf("ipv6 prefix contains ipv4 address: %s", p.Prefix)
		}
		out.V6 = append(out.V6, prefix)
	}
	return out, nil
}
// CheckIP checks if the given IP address is an AWS IP.
func (r *AWSIPRanges) CheckIP(ip netip.Addr) bool {
	if ip.IsLoopback() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() || ip.IsPrivate() {
		return false
	}
	if ip.Is4() {
		for _, p := range r.V4 {
			if p.Contains(ip) {
				return true
			}
		}
	} else {
		for _, p := range r.V6 {
			if p.Contains(ip) {
				return true
			}
		}
	}
	return false
}
