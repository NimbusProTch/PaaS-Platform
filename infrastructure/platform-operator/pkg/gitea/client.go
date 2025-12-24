package gitea

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// Client is a Gitea API and Git client
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	username   string
}

// Repository represents a Gitea repository
type Repository struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	HTMLURL     string `json:"html_url"`
	CloneURL    string `json:"clone_url"`
	SSHURL      string `json:"ssh_url"`
}

// CreateRepoOptions options for creating a repository
type CreateRepoOptions struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Private       bool   `json:"private"`
	AutoInit      bool   `json:"auto_init"`
	DefaultBranch string `json:"default_branch"`
}

// CommitFileOptions options for committing a file
type CommitFileOptions struct {
	Message     string
	Branch      string
	Author      string
	AuthorEmail string
	Content     string
}

// NewClient creates a new Gitea client
func NewClient(baseURL, username, token string) *Client {
	return &Client{
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		token:    token,
		username: username,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateOrganization creates a Gitea organization
func (c *Client) CreateOrganization(ctx context.Context, orgName, description string) error {
	body := map[string]interface{}{
		"username":    orgName,
		"description": description,
		"visibility":  "public",
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/orgs", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create organization: %w", err)
	}
	defer resp.Body.Close()

	// 201 Created or 422 if already exists
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusUnprocessableEntity {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// CreateRepository creates a repository in an organization
func (c *Client) CreateRepository(ctx context.Context, orgName string, opts CreateRepoOptions) (*Repository, error) {
	data, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/orgs/%s/repos", c.baseURL, orgName)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		// Check if repo already exists
		if resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusUnprocessableEntity {
			// Try to get existing repo
			return c.GetRepository(ctx, orgName, opts.Name)
		}
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var repo Repository
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &repo, nil
}

// GetRepository gets a repository
func (c *Client) GetRepository(ctx context.Context, orgName, repoName string) (*Repository, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s", c.baseURL, orgName, repoName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("repository not found: %s/%s", orgName, repoName)
	}

	var repo Repository
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &repo, nil
}

// PushFiles pushes multiple files to a repository
func (c *Client) PushFiles(ctx context.Context, repoURL, branch string, files map[string]string, commitMsg, authorName, authorEmail string) error {
	// Clone repository to temp directory with unique name (using nanosecond for uniqueness)
	tempDir := fmt.Sprintf("/tmp/gitea-repo-%d", time.Now().UnixNano())
	defer os.RemoveAll(tempDir) // Cleanup temp directory after push

	repo, err := git.PlainClone(tempDir, false, &git.CloneOptions{
		URL: repoURL,
		Auth: &githttp.BasicAuth{
			Username: c.username,
			Password: c.token,
		},
		ReferenceName: plumbing.ReferenceName("refs/heads/" + branch),
		SingleBranch:  true,
	})
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Write all files
	for path, content := range files {
		fullPath := fmt.Sprintf("%s/%s", tempDir, path)
		if err := ensureDir(fullPath); err != nil {
			return fmt.Errorf("failed to ensure directory: %w", err)
		}

		if err := writeFile(fullPath, content); err != nil {
			return fmt.Errorf("failed to write file %s: %w", path, err)
		}

		if _, err := w.Add(path); err != nil {
			return fmt.Errorf("failed to add file %s: %w", path, err)
		}
	}

	// Commit
	_, err = w.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	// Push
	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth: &githttp.BasicAuth{
			Username: c.username,
			Password: c.token,
		},
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch)),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}

	return nil
}

// GetBaseURL returns the base URL of the Gitea server
func (c *Client) GetBaseURL() string {
	return c.baseURL
}

// ConstructCloneURL constructs the internal clone URL for a repository
// This ensures we use the cluster-internal URL instead of the external ROOT_URL from Gitea API
func (c *Client) ConstructCloneURL(orgName, repoName string) string {
	return fmt.Sprintf("%s/%s/%s.git", c.baseURL, orgName, repoName)
}

// PullOCIChartAndExtract pulls a Helm chart from an OCI registry and extracts all files
// Returns a map of file paths to file contents
func (c *Client) PullOCIChartAndExtract(ctx context.Context, chartURL, version string) (map[string]string, error) {
	// Create temporary directory for chart download
	tmpDir, err := os.MkdirTemp("", "oci-chart-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Prepare helm pull command
	args := []string{"pull", chartURL}
	if version != "" {
		args = append(args, "--version", version)
	}
	args = append(args, "--destination", tmpDir, "--untar")

	// Execute helm pull
	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("helm pull failed: %w\nOutput: %s", err, string(output))
	}

	// Find the extracted chart directory
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read temp dir: %w", err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no chart extracted")
	}

	chartDir := filepath.Join(tmpDir, entries[0].Name())

	// Extract all files from the chart
	files := make(map[string]string)
	err = filepath.Walk(chartDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and Chart.yaml at root (we don't need it in Gitea)
		if info.IsDir() {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		// Calculate relative path from chart root
		relPath, err := filepath.Rel(chartDir, filePath)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Store with forward slashes for Git compatibility
		files[filepath.ToSlash(relPath)] = string(content)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk chart directory: %w", err)
	}

	return files, nil
}

// CloneAndExtractFiles clones a Git repository and extracts all files from a specific path
// Returns a map of file paths to file contents
func (c *Client) CloneAndExtractFiles(ctx context.Context, repoURL, branch, subPath string) (map[string]string, error) {
	// Create temporary directory for cloning
	tmpDir, err := os.MkdirTemp("", "charts-clone-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Clone options
	cloneOpts := &git.CloneOptions{
		URL:      repoURL,
		Progress: nil,
		Depth:    1, // Shallow clone for faster performance
	}

	// Set branch if specified
	if branch != "" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(branch)
		cloneOpts.SingleBranch = true
	}

	// Clone the repository
	_, err = git.PlainCloneContext(ctx, tmpDir, false, cloneOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	// Determine the source path
	sourcePath := tmpDir
	if subPath != "" {
		sourcePath = filepath.Join(tmpDir, subPath)
	}

	// Extract all files from the source path
	files := make(map[string]string)
	err = filepath.Walk(sourcePath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip .git directory
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		// Calculate relative path from source
		relPath, err := filepath.Rel(sourcePath, filePath)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Store with forward slashes for Git compatibility
		files[filepath.ToSlash(relPath)] = string(content)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return files, nil
}

// Helper functions
func ensureDir(filePath string) error {
	dir := filepath.Dir(filePath)
	return os.MkdirAll(dir, 0755)
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
