package p2p

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// UPnP error codes
var (
	ErrUPnPNotFound      = errors.New("upnp: no IGD device found")
	ErrUPnPTimeout       = errors.New("upnp: discovery timeout")
	ErrUPnPMappingFailed = errors.New("upnp: port mapping failed")
)

// UPnPClient handles UPnP IGD (Internet Gateway Device) operations
type UPnPClient struct {
	mu sync.Mutex

	gateway     *Gateway
	localIP     net.IP
	externalIP  net.IP
	logger      *slog.Logger
	discovered  bool
	mappings    map[int]*PortMapping // port -> mapping
}

// Gateway represents a UPnP IGD gateway
type Gateway struct {
	Host        string
	Port        int
	ControlURL  string
	ServiceType string
}

// PortMapping represents an active port mapping
type PortMapping struct {
	InternalPort int
	ExternalPort int
	Protocol     string
	Description  string
	LeaseDuration int
	CreatedAt    time.Time
}

// NewUPnPClient creates a new UPnP client
func NewUPnPClient(logger *slog.Logger) *UPnPClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &UPnPClient{
		logger:   logger,
		mappings: make(map[int]*PortMapping),
	}
}

// Discover finds UPnP IGD devices on the network
func (c *UPnPClient) Discover(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.discovered && c.gateway != nil {
		return nil
	}

	// Get local IP
	localIP, err := getOutboundIP()
	if err != nil {
		return fmt.Errorf("upnp: get local ip: %w", err)
	}
	c.localIP = localIP

	// SSDP M-SEARCH for IGD
	gateway, err := c.ssdpDiscover(ctx)
	if err != nil {
		return err
	}

	c.gateway = gateway
	c.discovered = true

	// Get external IP
	extIP, err := c.getExternalIP()
	if err != nil {
		c.logger.Warn("upnp: failed to get external IP", "err", err)
	} else {
		c.externalIP = extIP
		c.logger.Info("upnp: discovered gateway",
			"gateway", gateway.Host,
			"external_ip", extIP)
	}

	return nil
}

// ssdpDiscover performs SSDP discovery for UPnP IGD
func (c *UPnPClient) ssdpDiscover(ctx context.Context) (*Gateway, error) {
	// SSDP multicast address
	ssdpAddr := &net.UDPAddr{
		IP:   net.ParseIP("239.255.255.250"),
		Port: 1900,
	}

	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, fmt.Errorf("upnp: listen: %w", err)
	}
	defer conn.Close()

	// Search for WANIPConnection or WANPPPConnection
	searches := []string{
		"urn:schemas-upnp-org:service:WANIPConnection:1",
		"urn:schemas-upnp-org:service:WANIPConnection:2",
		"urn:schemas-upnp-org:service:WANPPPConnection:1",
	}

	for _, st := range searches {
		msg := fmt.Sprintf(
			"M-SEARCH * HTTP/1.1\r\n"+
				"HOST: 239.255.255.250:1900\r\n"+
				"ST: %s\r\n"+
				"MAN: \"ssdp:discover\"\r\n"+
				"MX: 2\r\n"+
				"\r\n", st)

		_, err = conn.WriteToUDP([]byte(msg), ssdpAddr)
		if err != nil {
			continue
		}
	}

	// Read responses
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			continue
		}

		response := string(buf[:n])
		gateway, err := c.parseSSDP(response)
		if err == nil && gateway != nil {
			return gateway, nil
		}
	}

	return nil, ErrUPnPNotFound
}

// parseSSDP parses an SSDP response and fetches device description
func (c *UPnPClient) parseSSDP(response string) (*Gateway, error) {
	// Find LOCATION header
	var location string
	for _, line := range strings.Split(response, "\r\n") {
		if strings.HasPrefix(strings.ToUpper(line), "LOCATION:") {
			location = strings.TrimSpace(line[9:])
			break
		}
	}

	if location == "" {
		return nil, errors.New("no location in SSDP response")
	}

	// Fetch device description with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", location, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // Limit 1MB
	if err != nil {
		return nil, err
	}

	return c.parseDeviceDescription(location, body)
}

// XML structures for UPnP device description
type upnpRoot struct {
	Device upnpDevice `xml:"device"`
}

type upnpDevice struct {
	DeviceType   string         `xml:"deviceType"`
	DeviceList   []upnpDevice   `xml:"deviceList>device"`
	ServiceList  []upnpService  `xml:"serviceList>service"`
}

type upnpService struct {
	ServiceType string `xml:"serviceType"`
	ControlURL  string `xml:"controlURL"`
}

// XML structures for SOAP responses
type soapEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    soapBody `xml:"Body"`
}

type soapBody struct {
	GetExternalIPResponse *getExternalIPResponse `xml:"GetExternalIPAddressResponse"`
}

type getExternalIPResponse struct {
	ExternalIP string `xml:"NewExternalIPAddress"`
}

// parseDeviceDescription parses UPnP device XML
func (c *UPnPClient) parseDeviceDescription(baseURL string, data []byte) (*Gateway, error) {
	var root upnpRoot
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	// Find WANIPConnection or WANPPPConnection service
	service := findService(&root.Device)
	if service == nil {
		return nil, errors.New("no WAN connection service found")
	}

	// Parse base URL using net/url
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid location URL: %w", err)
	}

	host := parsedURL.Hostname()
	port := 80
	if parsedURL.Port() != "" {
		_, _ = fmt.Sscanf(parsedURL.Port(), "%d", &port)
	}

	// Build control URL - resolve relative to base
	controlURL := service.ControlURL
	if !strings.HasPrefix(controlURL, "http") {
		controlRef, err := url.Parse(controlURL)
		if err == nil {
			controlURL = parsedURL.ResolveReference(controlRef).String()
		} else {
			// Fallback to manual construction
			if !strings.HasPrefix(controlURL, "/") {
				controlURL = "/" + controlURL
			}
			controlURL = fmt.Sprintf("%s://%s%s", parsedURL.Scheme, parsedURL.Host, controlURL)
		}
	}

	return &Gateway{
		Host:        host,
		Port:        port,
		ControlURL:  controlURL,
		ServiceType: service.ServiceType,
	}, nil
}

// findService recursively finds WAN connection service
func findService(device *upnpDevice) *upnpService {
	for i := range device.ServiceList {
		st := device.ServiceList[i].ServiceType
		if strings.Contains(st, "WANIPConnection") || strings.Contains(st, "WANPPPConnection") {
			return &device.ServiceList[i]
		}
	}

	for i := range device.DeviceList {
		if svc := findService(&device.DeviceList[i]); svc != nil {
			return svc
		}
	}

	return nil
}

// AddPortMapping adds a port mapping
func (c *UPnPClient) AddPortMapping(externalPort, internalPort int, protocol, description string, leaseDuration int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.gateway == nil {
		return ErrUPnPNotFound
	}

	if protocol == "" {
		protocol = "UDP"
	}
	if description == "" {
		description = "Shuttle P2P"
	}
	if leaseDuration == 0 {
		leaseDuration = 3600 // 1 hour default
	}

	soap := fmt.Sprintf(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:AddPortMapping xmlns:u="%s">
<NewRemoteHost></NewRemoteHost>
<NewExternalPort>%d</NewExternalPort>
<NewProtocol>%s</NewProtocol>
<NewInternalPort>%d</NewInternalPort>
<NewInternalClient>%s</NewInternalClient>
<NewEnabled>1</NewEnabled>
<NewPortMappingDescription>%s</NewPortMappingDescription>
<NewLeaseDuration>%d</NewLeaseDuration>
</u:AddPortMapping>
</s:Body>
</s:Envelope>`, c.gateway.ServiceType, externalPort, protocol, internalPort, c.localIP.String(), description, leaseDuration)

	action := fmt.Sprintf("%s#AddPortMapping", c.gateway.ServiceType)
	_, err := c.soapRequest(action, soap)
	if err != nil {
		return fmt.Errorf("upnp: add mapping: %w", err)
	}

	c.mappings[externalPort] = &PortMapping{
		InternalPort:  internalPort,
		ExternalPort:  externalPort,
		Protocol:      protocol,
		Description:   description,
		LeaseDuration: leaseDuration,
		CreatedAt:     time.Now(),
	}

	c.logger.Info("upnp: port mapping added",
		"external", externalPort,
		"internal", internalPort,
		"protocol", protocol)

	return nil
}

// DeletePortMapping removes a port mapping
func (c *UPnPClient) DeletePortMapping(externalPort int, protocol string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.gateway == nil {
		return ErrUPnPNotFound
	}

	if protocol == "" {
		protocol = "UDP"
	}

	soap := fmt.Sprintf(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:DeletePortMapping xmlns:u="%s">
<NewRemoteHost></NewRemoteHost>
<NewExternalPort>%d</NewExternalPort>
<NewProtocol>%s</NewProtocol>
</u:DeletePortMapping>
</s:Body>
</s:Envelope>`, c.gateway.ServiceType, externalPort, protocol)

	action := fmt.Sprintf("%s#DeletePortMapping", c.gateway.ServiceType)
	_, err := c.soapRequest(action, soap)
	if err != nil {
		return fmt.Errorf("upnp: delete mapping: %w", err)
	}

	delete(c.mappings, externalPort)

	c.logger.Info("upnp: port mapping deleted", "external", externalPort)
	return nil
}

// getExternalIP retrieves external IP from gateway
func (c *UPnPClient) getExternalIP() (net.IP, error) {
	if c.gateway == nil {
		return nil, ErrUPnPNotFound
	}

	soap := fmt.Sprintf(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:GetExternalIPAddress xmlns:u="%s">
</u:GetExternalIPAddress>
</s:Body>
</s:Envelope>`, c.gateway.ServiceType)

	action := fmt.Sprintf("%s#GetExternalIPAddress", c.gateway.ServiceType)
	resp, err := c.soapRequest(action, soap)
	if err != nil {
		return nil, err
	}

	// Parse SOAP response using XML unmarshaling
	var envelope soapEnvelope
	if err := xml.Unmarshal([]byte(resp), &envelope); err != nil {
		return nil, fmt.Errorf("parse external IP response: %w", err)
	}

	if envelope.Body.GetExternalIPResponse == nil {
		return nil, errors.New("no external IP in response")
	}

	ipStr := envelope.Body.GetExternalIPResponse.ExternalIP
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP: %s", ipStr)
	}

	return ip, nil
}

// soapRequest sends a SOAP request to the gateway
func (c *UPnPClient) soapRequest(action, body string) (string, error) {
	req, err := http.NewRequest("POST", c.gateway.ControlURL, strings.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "text/xml; charset=\"utf-8\"")
	req.Header.Set("SOAPAction", fmt.Sprintf("\"%s\"", action))

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("upnp: http %d: %s", resp.StatusCode, string(respBody))
	}

	return string(respBody), nil
}

// ExternalIP returns the external IP address
func (c *UPnPClient) ExternalIP() net.IP {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.externalIP
}

// LocalIP returns the local IP address
func (c *UPnPClient) LocalIP() net.IP {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.localIP
}

// IsAvailable returns whether UPnP is available
func (c *UPnPClient) IsAvailable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.discovered && c.gateway != nil
}

// GetMappings returns all active mappings
func (c *UPnPClient) GetMappings() map[int]*PortMapping {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[int]*PortMapping)
	for k, v := range c.mappings {
		result[k] = v
	}
	return result
}

// Close removes all port mappings
func (c *UPnPClient) Close() error {
	c.mu.Lock()
	ports := make([]int, 0, len(c.mappings))
	for port := range c.mappings {
		ports = append(ports, port)
	}
	c.mu.Unlock()

	for _, port := range ports {
		_ = c.DeletePortMapping(port, "UDP")
	}

	return nil
}

// RefreshMappings renews mappings that are about to expire
func (c *UPnPClient) RefreshMappings() error {
	c.mu.Lock()
	toRefresh := make([]*PortMapping, 0)
	for _, m := range c.mappings {
		// Refresh if less than 5 minutes remaining
		if time.Since(m.CreatedAt) > time.Duration(m.LeaseDuration-300)*time.Second {
			toRefresh = append(toRefresh, m)
		}
	}
	c.mu.Unlock()

	for _, m := range toRefresh {
		if err := c.AddPortMapping(m.ExternalPort, m.InternalPort, m.Protocol, m.Description, m.LeaseDuration); err != nil {
			c.logger.Warn("upnp: refresh failed", "port", m.ExternalPort, "err", err)
		}
	}

	return nil
}

// getOutboundIP gets the preferred outbound IP
func getOutboundIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP, nil
}

// PortMapper provides high-level port mapping with UPnP, NAT-PMP, and PCP
type PortMapper struct {
	upnp       *UPnPClient
	natpmp     *NATPMPClient
	pcp        *PCPClient
	logger     *slog.Logger
	mappedPort int
	localPort  int
	protocol   string // "upnp", "nat-pmp", "pcp", or ""
	mu         sync.Mutex
}

// NewPortMapper creates a new port mapper
func NewPortMapper(logger *slog.Logger) *PortMapper {
	if logger == nil {
		logger = slog.Default()
	}
	return &PortMapper{
		upnp:   NewUPnPClient(logger),
		natpmp: NewNATPMPClient(logger),
		pcp:    NewPCPClient(nil, logger),
		logger: logger,
	}
}

// MapPort attempts to map a port using UPnP, NAT-PMP, or PCP
// If preferredPort is 0, uses the same port as localPort
// Tries all protocols in parallel for faster discovery
func (pm *PortMapper) MapPort(ctx context.Context, localPort, preferredPort int) (int, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if preferredPort == 0 {
		preferredPort = localPort
	}

	// Try UPnP, NAT-PMP, and PCP in parallel for faster discovery
	type result struct {
		port     int
		protocol string
		err      error
	}

	results := make(chan result, 3)
	done := make(chan struct{})

	// Try UPnP
	go func() {
		port, err := pm.tryUPnP(ctx, localPort, preferredPort)
		select {
		case results <- result{port: port, protocol: "upnp", err: err}:
		case <-done:
		}
	}()

	// Try NAT-PMP
	go func() {
		port, err := pm.tryNATPMP(ctx, localPort, preferredPort)
		select {
		case results <- result{port: port, protocol: "nat-pmp", err: err}:
		case <-done:
		}
	}()

	// Try PCP
	go func() {
		port, err := pm.tryPCP(ctx, localPort, preferredPort)
		select {
		case results <- result{port: port, protocol: "pcp", err: err}:
		case <-done:
		}
	}()

	// Wait for results, use first success
	var lastErr error
	for i := 0; i < 3; i++ {
		select {
		case <-ctx.Done():
			close(done)
			return 0, ctx.Err()
		case r := <-results:
			if r.err == nil {
				close(done) // Signal other goroutines to exit
				pm.mappedPort = r.port
				pm.localPort = localPort
				pm.protocol = r.protocol
				pm.logger.Debug("port mapping succeeded", "protocol", r.protocol, "port", r.port)
				return r.port, nil
			}
			lastErr = r.err
		}
	}

	close(done)
	if lastErr != nil {
		return 0, lastErr
	}
	return 0, ErrUPnPMappingFailed
}

// tryUPnP attempts port mapping via UPnP
func (pm *PortMapper) tryUPnP(ctx context.Context, localPort, preferredPort int) (int, error) {
	if err := pm.upnp.Discover(ctx); err != nil {
		return 0, err
	}

	// Try preferred port first
	err := pm.upnp.AddPortMapping(preferredPort, localPort, "UDP", "Shuttle P2P", 3600)
	if err == nil {
		return preferredPort, nil
	}

	// Try alternative ports
	for _, port := range []int{preferredPort + 1, preferredPort + 2, localPort, localPort + 1} {
		if port == preferredPort {
			continue
		}
		err = pm.upnp.AddPortMapping(port, localPort, "UDP", "Shuttle P2P", 3600)
		if err == nil {
			return port, nil
		}
	}

	return 0, ErrUPnPMappingFailed
}

// tryNATPMP attempts port mapping via NAT-PMP
func (pm *PortMapper) tryNATPMP(ctx context.Context, localPort, preferredPort int) (int, error) {
	// Check context before starting
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	if err := pm.natpmp.Discover(); err != nil {
		return 0, err
	}

	// Check context after discovery
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	// Try preferred port first
	port, err := pm.natpmp.AddPortMapping(localPort, preferredPort, "UDP", 3600)
	if err == nil {
		return port, nil
	}

	// Try alternative ports
	for _, extPort := range []int{preferredPort + 1, preferredPort + 2, localPort, localPort + 1} {
		if extPort == preferredPort {
			continue
		}
		port, err = pm.natpmp.AddPortMapping(localPort, extPort, "UDP", 3600)
		if err == nil {
			return port, nil
		}
	}

	return 0, ErrNATPMPMappingFailed
}

// tryPCP attempts port mapping via PCP
func (pm *PortMapper) tryPCP(ctx context.Context, localPort, preferredPort int) (int, error) {
	// Check context before starting
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	if err := pm.pcp.Discover(); err != nil {
		return 0, err
	}

	// Check context after discovery
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	// Try preferred port first
	mapping, err := pm.pcp.AddPortMapping(protocolUDP, localPort, preferredPort, time.Hour)
	if err == nil {
		return mapping.ExternalPort, nil
	}

	// Try alternative ports
	for _, extPort := range []int{preferredPort + 1, preferredPort + 2, localPort, localPort + 1} {
		if extPort == preferredPort {
			continue
		}
		mapping, err = pm.pcp.AddPortMapping(protocolUDP, localPort, extPort, time.Hour)
		if err == nil {
			return mapping.ExternalPort, nil
		}
	}

	return 0, errors.New("pcp: port mapping failed")
}

// GetMappedPort returns the mapped external port
func (pm *PortMapper) GetMappedPort() int {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.mappedPort
}

// GetExternalAddr returns the external address (IP:port)
func (pm *PortMapper) GetExternalAddr() *net.UDPAddr {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.mappedPort == 0 {
		return nil
	}

	var extIP net.IP
	switch pm.protocol {
	case "nat-pmp":
		extIP = pm.natpmp.ExternalIP()
	case "pcp":
		extIP = pm.pcp.ExternalIP()
	default:
		extIP = pm.upnp.ExternalIP()
	}

	if extIP == nil {
		return nil
	}

	return &net.UDPAddr{
		IP:   extIP,
		Port: pm.mappedPort,
	}
}

// Close removes the port mapping
func (pm *PortMapper) Close() error {
	pm.mu.Lock()
	port := pm.mappedPort
	localPort := pm.localPort
	protocol := pm.protocol
	pm.mappedPort = 0
	pm.localPort = 0
	pm.protocol = ""
	pm.mu.Unlock()

	if port > 0 {
		switch protocol {
		case "nat-pmp":
			return pm.natpmp.DeletePortMapping(localPort, "UDP")
		case "pcp":
			return pm.pcp.DeletePortMapping(localPort)
		default:
			return pm.upnp.DeletePortMapping(port, "UDP")
		}
	}
	return nil
}

// IsAvailable returns whether port mapping is available
func (pm *PortMapper) IsAvailable() bool {
	return pm.upnp.IsAvailable() || pm.natpmp.IsAvailable() || pm.pcp.IsAvailable()
}

// Refresh renews the port mapping
func (pm *PortMapper) Refresh() error {
	pm.mu.Lock()
	protocol := pm.protocol
	localPort := pm.localPort
	pm.mu.Unlock()

	switch protocol {
	case "nat-pmp":
		return pm.natpmp.RefreshMappings()
	case "pcp":
		return pm.pcp.RefreshMapping(localPort, time.Hour)
	default:
		return pm.upnp.RefreshMappings()
	}
}

// Protocol returns which protocol is being used ("upnp", "nat-pmp", "pcp", or "")
func (pm *PortMapper) Protocol() string {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.protocol
}
