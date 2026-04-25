package orchestrator

import (
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cloak"
)

type cloakSessionDetails struct {
	ProfileID string
	BaseURL   string
	WSURL     string
}

func createCloakSession(baseURL, profileID string) (*cloakSessionDetails, error) {
	client := cloak.NewClient(baseURL)
	launchResp, err := client.LaunchProfile(profileID)
	if err != nil {
		if !strings.Contains(err.Error(), "returned 409") {
			return nil, err
		}
		launchResp = &cloak.LaunchResponse{}
	}

	wsURL := strings.TrimSpace(launchResp.CDPURL)
	if strings.HasPrefix(wsURL, "ws://") || strings.HasPrefix(wsURL, "wss://") {
		return &cloakSessionDetails{
			ProfileID: strings.TrimSpace(profileID),
			BaseURL:   cloak.NormalizeBaseURL(baseURL),
			WSURL:     wsURL,
		}, nil
	}

	resolvedWS, err := client.BrowserWSEndpoint(profileID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(resolvedWS) == "" {
		return nil, fmt.Errorf("cloak session did not return a browser websocket endpoint")
	}
	return &cloakSessionDetails{
		ProfileID: strings.TrimSpace(profileID),
		BaseURL:   cloak.NormalizeBaseURL(baseURL),
		WSURL:     strings.TrimSpace(resolvedWS),
	}, nil
}

func stopCloakSession(baseURL, profileID string) {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(profileID) == "" {
		return
	}
	_ = cloak.NewClient(baseURL).StopProfile(profileID)
}
