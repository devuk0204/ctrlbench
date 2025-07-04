package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/devuk0204/ctrlbench/types"
)

// getCfgString returns cfg.<key>.value if the node is a map,
// otherwise tries to cast the raw node to string.
func getCfgString(node interface{}) (string, bool) {
	if m, ok := node.(map[string]interface{}); ok {
		if v, ok := m["value"].(string); ok {
			return v, true
		}
	}
	str, ok := node.(string)
	return str, ok
}

// trimSlashRight removes the trailing slash once.
func trimSlashRight(s string) string {
	return strings.TrimSuffix(s, "/")
}

type NFDiscoveryClient struct {
	NRFURL     string
	HTTPClient *http.Client
}

func NewNFDiscoveryClient(nrfURL string, timeout time.Duration) *NFDiscoveryClient {
	return &NFDiscoveryClient{
		NRFURL: trimSlashRight(nrfURL),
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *NFDiscoveryClient) DiscoverNF(
	targetNFType, requesterNFType, requesterNFInstanceID string,
) (*types.SearchResult, error) {

	base := fmt.Sprintf("%s/nnrf-disc/v1/nf-instances", c.NRFURL)

	q := url.Values{}
	q.Set("target-nf-type", strings.ToUpper(targetNFType))
	if requesterNFType != "" {
		q.Set("requester-nf-type", requesterNFType)
	}
	if requesterNFInstanceID != "" {
		q.Set("requester-nf-instance-id", requesterNFInstanceID)
	}

	fullURL := base + "?" + q.Encode()
	fmt.Printf("🔍 NRF discovery URL : %s\n", fullURL)

	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create discovery request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discovery request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("discovery failed (%d): %s", resp.StatusCode, b)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read discovery response: %w", err)
	}

	var res types.SearchResult
	if err = json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("parse discovery response: %w", err)
	}
	return &res, nil
}

func (c *NFDiscoveryClient) nfURLfromProfile(p types.NFProfile) (string, bool) {
	for _, svc := range p.NFServices {
		for _, ep := range svc.IpEndPoints {
			switch {
			case ep.IPv4Address != "" && ep.Port > 0:
				return fmt.Sprintf("http://%s:%d", ep.IPv4Address, ep.Port), true
			case ep.IPv6Address != "" && ep.Port > 0:
				return fmt.Sprintf("http://[%s]:%d", ep.IPv6Address, ep.Port), true
			}
		}
	}
	if len(p.IPv4Addresses) > 0 {
		return fmt.Sprintf("http://%s:80", p.IPv4Addresses[0]), true
	}
	if p.FQDN != "" {
		return fmt.Sprintf("http://%s:80", p.FQDN), true
	}
	return "", false
}

func (c *NFDiscoveryClient) DiscoverAndGetURL(
	targetNFType, requesterNFType, requesterNFInstanceID string,
) (string, error) {

	res, err := c.DiscoverNF(targetNFType, requesterNFType, requesterNFInstanceID)
	if err != nil {
		return "", err
	}
	if len(res.NFInstances) == 0 {
		return "", fmt.Errorf("no %s instances found", targetNFType)
	}

	// prefer REGISTERED → otherwise first instance
	for _, inst := range res.NFInstances {
		if inst.NFStatus == "REGISTERED" {
			if url, ok := c.nfURLfromProfile(inst); ok {
				return url, nil
			}
		}
	}
	if url, ok := c.nfURLfromProfile(res.NFInstances[0]); ok {
		fmt.Printf("⚠️  NF URL (fallback): %s\n", url)
		return url, nil
	}
	return "", fmt.Errorf("no suitable ipEndPoint found")
}

// NFDiscoveryURL is used by the benchmark runner.
// It reads human-friendly configuration nodes and launches discovery.
func NFDiscoveryURL(
	cfg map[string]interface{},
	targetNFType string,
) (string, error) {

	nrfURL, ok := getCfgString(cfg["nrf_url"])
	if !ok {
		return "", fmt.Errorf("nrf_url missing in configuration")
	}
	reqType, _ := getCfgString(cfg["requester_nf_type"])
	reqID, _ := getCfgString(cfg["requester_nf_instance_id"])

	client := NewNFDiscoveryClient(nrfURL, 10*time.Second)
	url, err := client.DiscoverAndGetURL(targetNFType, reqType, reqID)
	if err != nil {
		fmt.Printf("❌ NF discovery error: %v\n", err)
		return "", err
	}
	return url, nil
}
