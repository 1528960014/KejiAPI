package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kejiapi/common"
)

// Antigravity (Code Assist) endpoints. Generation uses the daily endpoint
// by default; discovery tries both.
const (
	antigravityProdEndpoint  = "https://cloudcode-pa.googleapis.com"
	antigravityDailyEndpoint = "https://daily-cloudcode-pa.googleapis.com"

	antigravityGCPProjectsURL = "https://cloudresourcemanager.googleapis.com/v1/projects"

	antigravityNodeUA       = "google-api-nodejs-client/10.3.0"
	antigravityAPICLientHdr = "gl-node/22.18.0"
	antigravityClientMeta   = `{"ideType":"IDE_UNSPECIFIED","platform":"PLATFORM_UNSPECIFIED","pluginType":"GEMINI"}`

	antigravityOnboardPolls = 30
	antigravityOnboardDelay = 2 * time.Second
)

type antigravityTierInfo struct {
	ID         string `json:"id"`
	IsDefault  bool   `json:"isDefault"`
	UserDefinedProject bool `json:"userDefinedCloudaicompanionProject"`
}

type antigravityLoadResponse struct {
	CurrentTier             struct {
		ID string `json:"id"`
	} `json:"currentTier"`
	CloudaicompanionProject json.RawMessage   `json:"cloudaicompanionProject"`
	AllowedTiers            []antigravityTierInfo `json:"allowedTiers"`
}

type antigravityLRO struct {
	Done bool `json:"done"`
	Response struct {
		CloudaicompanionProject json.RawMessage `json:"cloudaicompanionProject"`
	} `json:"response"`
}

func antigravityDiscoveryHeaders(accessToken string) http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+accessToken)
	h.Set("Content-Type", "application/json")
	h.Set("User-Agent", antigravityNodeUA)
	h.Set("X-Goog-Api-Client", antigravityAPICLientHdr)
	h.Set("Client-Metadata", antigravityClientMeta)
	return h
}

func antigravityCoreMetadata() map[string]any {
	return map[string]any{
		"ideType":      "IDE_UNSPECIFIED",
		"platform":     "PLATFORM_UNSPECIFIED",
		"pluginType":   "GEMINI",
	}
}

// extractAntigravityProjectID handles the API returning
// cloudaicompanionProject either as a plain string or as an object with id.
func extractAntigravityProjectID(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	var str string
	if err := common.Unmarshal([]byte(s), &str); err == nil {
		return strings.TrimSpace(str)
	}
	var obj struct {
		ID string `json:"id"`
	}
	if err := common.Unmarshal([]byte(s), &obj); err == nil {
		return strings.TrimSpace(obj.ID)
	}
	return ""
}

func postAntigravityJSON(ctx context.Context, client *http.Client, url string, headers http.Header, body any) ([]byte, int, error) {
	payload, err := common.Marshal(body)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	for k, vals := range headers {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

// callLoadCodeAssist probes one endpoint and returns the parsed response
// when the server answered 200.
func callLoadCodeAssist(ctx context.Context, client *http.Client, accessToken, endpoint, projectID string) (*antigravityLoadResponse, bool, error) {
	body := map[string]any{
		"cloudaicompanionProject": nil,
		"metadata":                antigravityCoreMetadata(),
	}
	if projectID = strings.TrimSpace(projectID); projectID != "" {
		body["cloudaicompanionProject"] = projectID
	}
	data, status, err := postAntigravityJSON(ctx, client, endpoint+":loadCodeAssist", antigravityDiscoveryHeaders(accessToken), body)
	if err != nil {
		return nil, false, err
	}
	if status != http.StatusOK {
		return nil, false, nil
	}
	var parsed antigravityLoadResponse
	if err := common.Unmarshal(data, &parsed); err != nil {
		return nil, false, err
	}
	return &parsed, true, nil
}

func pickAntigravityOnboardTier(tiers []antigravityTierInfo) string {
	for _, t := range tiers {
		if t.IsDefault {
			return t.ID
		}
	}
	for _, t := range tiers {
		if t.ID == "legacy-tier" {
			return t.ID
		}
	}
	if len(tiers) > 0 {
		return tiers[0].ID
	}
	return ""
}

// callOnboardUser runs the onboarding long-running operation (re-POST until
// done) and returns the resulting project id.
func callOnboardUser(ctx context.Context, client *http.Client, accessToken, tierID string) (string, error) {
	body := map[string]any{
		"tierId":                   tierID,
		"cloudaicompanionProject":  nil,
		"metadata":                 antigravityCoreMetadata(),
	}
	headers := antigravityDiscoveryHeaders(accessToken)

	var lastErr error
	for _, endpoint := range []string{antigravityDailyEndpoint, antigravityProdEndpoint} {
		actionURL := endpoint + ":onboardUser"
		data, status, err := postAntigravityJSON(ctx, client, actionURL, headers, body)
		if err != nil {
			lastErr = err
			continue
		}
		if status != http.StatusOK {
			lastErr = fmt.Errorf("onboardUser returned status %d", status)
			continue
		}
		var lro antigravityLRO
		if err := common.Unmarshal(data, &lro); err != nil {
			lastErr = err
			continue
		}

		for i := 0; !lro.Done && i < antigravityOnboardPolls-1; i++ {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(antigravityOnboardDelay):
			}
			data, status, err = postAntigravityJSON(ctx, client, actionURL, headers, body)
			if err != nil {
				lastErr = err
				break
			}
			if status != http.StatusOK {
				lastErr = fmt.Errorf("onboardUser poll returned status %d", status)
				break
			}
			if err := common.Unmarshal(data, &lro); err != nil {
				lastErr = err
				break
			}
		}

		if lro.Done {
			return extractAntigravityProjectID(lro.Response.CloudaicompanionProject), nil
		}
		if lastErr != nil {
			continue
		}
		lastErr = errors.New("onboarding timed out")
	}
	if lastErr == nil {
		lastErr = errors.New("all onboardUser endpoints failed")
	}
	return "", lastErr
}

func listFirstActiveGCPProject(ctx context.Context, client *http.Client, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, antigravityGCPProjectsURL+"?pageSize=100", nil)
	if err != nil {
		return "", err
	}
	for k, vals := range antigravityDiscoveryHeaders(accessToken) {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("gcp project list returned status %d", resp.StatusCode)
	}
	var payload struct {
		Projects []struct {
			ProjectID      string `json:"projectId"`
			LifecycleState string `json:"lifecycleState"`
		} `json:"projects"`
	}
	if err := common.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	for _, p := range payload.Projects {
		if p.LifecycleState == "ACTIVE" && strings.TrimSpace(p.ProjectID) != "" {
			return p.ProjectID, nil
		}
	}
	return "", errors.New("no active Google Cloud project found for this account")
}

// DiscoverAntigravityProject resolves the GCP project id (and tier) for a
// freshly authorized Antigravity credential and stores them on the
// credential. It follows the official discovery flow: loadCodeAssist →
// onboardUser (LRO) → GCP Resource Manager fallback.
func DiscoverAntigravityProject(ctx context.Context, cred *AntigravityCredential, proxyURL string) error {
	if cred == nil {
		return errors.New("credential is nil")
	}
	if strings.TrimSpace(cred.AccessToken) == "" {
		return errors.New("access_token is required for project discovery")
	}
	if strings.TrimSpace(cred.ProjectID) != "" {
		return nil
	}

	client, err := getCodexOAuthHTTPClient(proxyURL)
	if err != nil {
		return err
	}

	// 1) loadCodeAssist (prod first for better project resolution, then daily).
	var loadErr error
	for _, endpoint := range []string{antigravityProdEndpoint, antigravityDailyEndpoint} {
		load, ok, err := callLoadCodeAssist(ctx, client, cred.AccessToken, endpoint, "")
		if err != nil {
			loadErr = err
			continue
		}
		if !ok {
			continue
		}

		// Already onboarded and the server returned a project.
		if load.CurrentTier.ID != "" {
			if project := extractAntigravityProjectID(load.CloudaicompanionProject); project != "" {
				cred.ProjectID = project
				cred.Tier = load.CurrentTier.ID
				return nil
			}
		}

		// Needs onboarding: run the LRO on the daily endpoint first.
		if tierID := pickAntigravityOnboardTier(load.AllowedTiers); tierID != "" {
			project, err := callOnboardUser(ctx, client, cred.AccessToken, tierID)
			if err == nil && strings.TrimSpace(project) != "" {
				cred.ProjectID = project
				cred.Tier = tierID
				return nil
			}
			loadErr = err
		}
		// A 200 loadCodeAssist answered; stop probing endpoints.
		break
	}

	// 2) Fallback: first active GCP project of the account.
	project, err := listFirstActiveGCPProject(ctx, client, cred.AccessToken)
	if err != nil {
		if loadErr != nil {
			return fmt.Errorf("antigravity project discovery failed: %v (fallback: %v)", loadErr, err)
		}
		return fmt.Errorf("antigravity project discovery failed: %w", err)
	}
	cred.ProjectID = project
	return nil
}