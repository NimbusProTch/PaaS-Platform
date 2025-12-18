package clients

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type GitHubClient struct {
	token      string
	httpClient *http.Client
}

type GitHubRelease struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func NewGitHubClient(token string) *GitHubClient {
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	return &GitHubClient{
		token:      token,
		httpClient: &http.Client{},
	}
}

// GetRelease fetches release information from GitHub
func (c *GitHubClient) GetRelease(ctx context.Context, owner, repo, tag string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", owner, repo, tag)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &release, nil
}

// DownloadAsset downloads a release asset
func (c *GitHubClient) DownloadAsset(ctx context.Context, assetURL, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", assetURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/octet-stream")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	// Create destination file
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer out.Close()

	// Copy content
	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}

// ExtractTarGz extracts a tar.gz file
func (c *GitHubClient) ExtractTarGz(src, dest string) error {
	file, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		target := filepath.Join(dest, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("creating directory: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("creating directory: %w", err)
			}

			outFile, err := os.Create(target)
			if err != nil {
				return fmt.Errorf("creating file: %w", err)
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return fmt.Errorf("writing file: %w", err)
			}
			outFile.Close()

			if err := os.Chmod(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("setting file permissions: %w", err)
			}
		}
	}

	return nil
}

// GetManifests downloads and extracts Kubernetes manifests from a release
func (c *GitHubClient) GetManifests(ctx context.Context, owner, repo, tag string) (string, error) {
	// Parse owner/repo from repository string
	parts := strings.Split(strings.TrimPrefix(repo, "github.com/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid repository format: %s", repo)
	}

	actualOwner := parts[0]
	actualRepo := parts[1]

	// Get release
	release, err := c.GetRelease(ctx, actualOwner, actualRepo, tag)
	if err != nil {
		return "", fmt.Errorf("getting release: %w", err)
	}

	// Look for manifest asset (k8s.tar.gz or manifests.tar.gz)
	var manifestAsset *Asset
	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, "k8s") || strings.Contains(asset.Name, "manifest") {
			manifestAsset = &asset
			break
		}
	}

	if manifestAsset == nil {
		// If no manifest asset, return path to k8s directory
		return fmt.Sprintf("/tmp/releases/%s/%s/%s/k8s", actualOwner, actualRepo, tag), nil
	}

	// Download and extract manifest
	tempDir := fmt.Sprintf("/tmp/releases/%s/%s/%s", actualOwner, actualRepo, tag)
	assetPath := filepath.Join(tempDir, manifestAsset.Name)

	if err := c.DownloadAsset(ctx, manifestAsset.BrowserDownloadURL, assetPath); err != nil {
		return "", fmt.Errorf("downloading asset: %w", err)
	}

	if strings.HasSuffix(manifestAsset.Name, ".tar.gz") || strings.HasSuffix(manifestAsset.Name, ".tgz") {
		if err := c.ExtractTarGz(assetPath, tempDir); err != nil {
			return "", fmt.Errorf("extracting archive: %w", err)
		}
	}

	return tempDir, nil
}