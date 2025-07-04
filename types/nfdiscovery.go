package types

type IpEndPoint struct {
	IPv4Address string `json:"ipv4Address,omitempty"`
	IPv6Address string `json:"ipv6Address,omitempty"`
	Port        int    `json:"port,omitempty"`
	Transport   string `json:"transport,omitempty"`
}

type NFProfile struct {
	NFInstanceID  string       `json:"nfInstanceId"`
	NFType        string       `json:"nfType"`
	NFStatus      string       `json:"nfStatus"`
	FQDN          string       `json:"fqdn,omitempty"`
	IPv4Addresses []string     `json:"ipv4Addresses,omitempty"`
	IpEndPoints   []IpEndPoint `json:"ipEndPoints,omitempty"`
	NFServices    []NFService  `json:"nfServices,omitempty"`
}

type NFService struct {
	ServiceInstanceID string      `json:"serviceInstanceId"`
	ServiceName       string      `json:"serviceName"`
	Versions          []Version   `json:"versions,omitempty"`
	Scheme            string      `json:"scheme,omitempty"`
	FQDN              string      `json:"fqdn,omitempty"`
	IPv4Addresses     []string    `json:"ipv4Addresses,omitempty"`
	IpEndPoints       []IpEndPoint `json:"ipEndPoints,omitempty"`
	Port              int         `json:"port,omitempty"`
	APIPrefix         string      `json:"apiPrefix,omitempty"`
}

type Version struct {
	APIVersionInURI string `json:"apiVersionInUri"`
	APIFullVersion  string `json:"apiFullVersion,omitempty"`
}

type SearchResult struct {
	ValidityPeriod int         `json:"validityPeriod,omitempty"`
	NFInstances    []NFProfile `json:"nfInstances"`
}
