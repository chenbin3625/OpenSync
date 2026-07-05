package service

import (
	"net/http"
	"net/url"
	"opensync/internal/config"
	"time"
)

func newOutboundTransport(maxIdleConns, maxIdleConnsPerHost int, idleConnTimeout time.Duration) *http.Transport {
	return &http.Transport{
		Proxy:               configuredProxy,
		MaxIdleConns:        maxIdleConns,
		MaxIdleConnsPerHost: maxIdleConnsPerHost,
		IdleConnTimeout:     idleConnTimeout,
	}
}

func configuredProxy(_ *http.Request) (*url.URL, error) {
	proxyURL := config.GetConfig().Server.ProxyURL
	proxyURL, err := config.NormalizeProxyURL(proxyURL)
	if err != nil || proxyURL == "" {
		return nil, err
	}
	return url.Parse(proxyURL)
}

func outboundProxyConfigured() bool {
	proxyURL, err := config.NormalizeProxyURL(config.GetConfig().Server.ProxyURL)
	return err == nil && proxyURL != ""
}

// RefreshOutboundHTTPClients closes idle connections so a changed proxy
// setting is picked up promptly by already cached HTTP clients.
func RefreshOutboundHTTPClients() {
	notifyHTTPClient.CloseIdleConnections()

	alistClientListMu.RLock()
	clients := make([]*AlistClient, 0, len(alistClientList))
	for _, client := range alistClientList {
		clients = append(clients, client)
	}
	alistClientListMu.RUnlock()

	for _, client := range clients {
		client.Close()
	}
}
