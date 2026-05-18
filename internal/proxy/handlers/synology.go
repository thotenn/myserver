package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/thotenn/myserver/internal/config"
	"github.com/thotenn/myserver/internal/proxy"
)

// SynologyProxyHandler handles Synology DiskStation/DownloadStation APIs.
// Uses SID-based authentication.
func SynologyProxyHandler(ctx context.Context, widget *config.WidgetConfig, endpoint string) (interface{}, error) {
	// Step 1: Login to get SID
	loginURL := fmt.Sprintf("%s/webapi/auth.cgi?api=SYNO.API.Auth&version=6&method=login&account=%s&passwd=%s&format=sid",
		widget.URL, widget.Username, widget.Password,
	)

	loginResult, err := proxy.Proxy(ctx, loginURL, &proxy.Params{})
	if err != nil {
		return nil, fmt.Errorf("synology login: %w", err)
	}

	var loginResp struct {
		Success bool `json:"success"`
		Data    struct {
			SID string `json:"sid"`
		} `json:"data"`
	}
	if err := json.Unmarshal(loginResult.Body, &loginResp); err != nil {
		return nil, fmt.Errorf("parsing synology login response: %w", err)
	}
	if !loginResp.Success {
		return nil, fmt.Errorf("synology login failed")
	}

	// Step 2: Make API request with SID
	apiURL := fmt.Sprintf("%s/webapi/%s&_sid=%s", widget.URL, endpoint, loginResp.Data.SID)
	result, err := proxy.Proxy(ctx, apiURL, &proxy.Params{})
	if err != nil {
		return nil, err
	}

	// Step 3: Logout
	logoutURL := fmt.Sprintf("%s/webapi/auth.cgi?api=SYNO.API.Auth&version=6&method=logout&_sid=%s",
		widget.URL, loginResp.Data.SID,
	)
	go proxy.Proxy(ctx, logoutURL, &proxy.Params{})

	if result.Status != http.StatusOK {
		return nil, fmt.Errorf("synology API returned status %d", result.Status)
	}

	var data interface{}
	if err := json.Unmarshal(result.Body, &data); err != nil {
		return nil, fmt.Errorf("parsing synology response: %w", err)
	}

	return data, nil
}
