package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"gopkg.in/yaml.v2"
)

const (
	DefaultGitHubOrg  = "NimbusProTch"
	DefaultGitHubRepo = "PaaS-Platform"
)

// ReleaseInfo represents the release-info.yaml metadata
type ReleaseInfo struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
		Tag     string `yaml:"tag"`
	} `yaml:"metadata"`
	Spec struct {
		Image      string `yaml:"image"`
		Commit     string `yaml:"commit"`
		BuildDate  string `yaml:"buildDate"`
		Repository string `yaml:"repository"`
	} `yaml:"spec"`
}

// Client handles GitHub API interactions
type Client struct {
	httpClient *http.Client
	org        string
	repo       string
	token      string // Optional: for private repos or higher rate limits
}

// NewClient creates a new GitHub client
func NewClient(org, repo, token string) *Client {
	if org == "" {
		org = DefaultGitHubOrg
	}
	if repo == "" {
		repo = DefaultGitHubRepo
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		org:   org,
		repo:  repo,
		token: token,
	}
}

// GetReleaseInfo fetches release-info.yaml from a GitHub release
func (c *Client) GetReleaseInfo(ctx context.Context, serviceName, version string) (*ReleaseInfo, error) {
	// Construct tag name: service-name-vX.Y.Z
	tag := fmt.Sprintf("%s-%s", serviceName, version)

	// GitHub release asset URL
	url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/release-info.yaml",
		c.org, c.repo, tag)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch release info: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var releaseInfo ReleaseInfo
	if err := yaml.Unmarshal(body, &releaseInfo); err != nil {
		return nil, fmt.Errorf("failed to parse release info: %w", err)
	}

	return &releaseInfo, nil
}

// GitHubRelease represents a GitHub release
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// GetRelease fetches release metadata from GitHub API
func (c *Client) GetRelease(ctx context.Context, tag string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s",
		c.org, c.repo, tag)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch release: status %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode release: %w", err)
	}

	return &release, nil
}

// ResolveImageURL resolves the image URL from GitHub release
// If serviceName and version are provided, fetches from release-info.yaml
// Otherwise falls back to direct image specification
func (c *Client) ResolveImageURL(ctx context.Context, serviceName, version, fallbackImage string) (string, error) {
	// If no serviceName/version, use fallback image
	if serviceName == "" || version == "" {
		if fallbackImage == "" {
			return "", fmt.Errorf("either serviceName+version or image must be specified")
		}
		return fallbackImage, nil
	}

	// Fetch from GitHub release
	releaseInfo, err := c.GetReleaseInfo(ctx, serviceName, version)
	if err != nil {
		// If GitHub fetch fails and fallback exists, use it
		if fallbackImage != "" {
			return fallbackImage, nil
		}
		return "", fmt.Errorf("failed to resolve image from GitHub release: %w", err)
	}

	if releaseInfo.Spec.Image == "" {
		return "", fmt.Errorf("release info does not contain image URL")
	}

	return releaseInfo.Spec.Image, nil
}
