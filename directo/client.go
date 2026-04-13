// Package directo provides a Go client for the Directo accounting API.
//
// Directo uses two separate APIs:
//   - REST API (read-only): JSON responses, API key authentication
//   - XML Direct API (read+write): XML format, token authentication
//
// This client manages both APIs transparently.
package directo

import (
	"fmt"
	"net/http"
)

const (
	DefaultRESTBaseURL = "https://login.directo.ee/apidirect/v1/"
	// XML Direct endpoint — used for both reads and writes. The routing
	// params (what, get/put, filters) MUST go in the URL query string;
	// only token and xmldata belong in the POST body. Posting routing
	// params in the body returns <result type="404" desc="Invalid url given"/>.
	DefaultXMLBaseURL = "https://login.directo.ee/xmlcore/cap_xml_direct/xmlcore.asp"
)

// Config holds the configuration for a Directo API client.
type Config struct {
	// Company is the Directo company/database code (used in XML Direct URL path).
	Company string

	// Token is the XML Direct authentication token (for write operations).
	Token string

	// RestAPIKey is the REST API key (for read operations via X-Directo-Key header).
	RestAPIKey string

	// XMLBaseURL overrides the default XML Direct endpoint.
	// Defaults to https://login.directo.ee/xmlcore/cap_xml_direct/xmlcore.asp
	XMLBaseURL string

	// HTTPClient is an optional HTTP client for making requests.
	// Defaults to http.DefaultClient if nil.
	HTTPClient *http.Client
}

// Client is a Directo API client that manages both REST and XML Direct APIs.
type Client struct {
	rest *restClient
	xml  *xmlClient
}

// New creates a new Directo API client with the given configuration.
func New(cfg Config) (*Client, error) {
	if cfg.Company == "" {
		return nil, fmt.Errorf("directo: company code is required")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("directo: XML Direct token is required")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	xmlBaseURL := cfg.XMLBaseURL
	if xmlBaseURL == "" {
		xmlBaseURL = DefaultXMLBaseURL
	}

	return &Client{
		rest: &restClient{
			baseURL:    DefaultRESTBaseURL,
			apiKey:     cfg.RestAPIKey,
			httpClient: httpClient,
		},
		xml: &xmlClient{
			baseURL:    xmlBaseURL,
			token:      cfg.Token,
			httpClient: httpClient,
		},
	}, nil
}
