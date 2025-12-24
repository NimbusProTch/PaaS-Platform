package gitea

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
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
	Message   string
	Branch    string
	Author    string
	AuthorEmail string
	Content   string
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
	// Clone repository to temp directory
	tempDir := fmt.Sprintf("/tmp/gitea-repo-%d", time.Now().Unix())

	repo, err := git.PlainClone(tempDir, false, &git.CloneOptions{
		URL: repoURL,
		Auth: &githttp.BasicAuth{
			Username: c.username,
			Password: c.token,
		},
		ReferenceName: "refs/heads/" + branch,
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

// Helper functions
func ensureDir(filePath string) error {
	// Implementation to ensure directory exists
	// This is a placeholder - actual implementation would use os.MkdirAll
	return nil
}

func writeFile(path, content string) error {
	// Implementation to write file
	// This is a placeholder - actual implementation would use os.WriteFile
	return nil
}
