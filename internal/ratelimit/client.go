package ratelimit

import (
	"context"
	"errors"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/google/go-github/v32/github"
)

// Client is a rate-limiting Github client which blocks API requests that would exceed the rate limit.
// Any requests that would exceed the rate limit block until the rate limit resets. It only implements the
// github APIs needed by this project.
type Client struct {
	ctx    context.Context
	client *github.Client

	mut     sync.Mutex
	limited bool
	resetAt time.Time
	count   int // used to count API calls
}

// NewClient constructs a new client using the provided arguments.
func NewClient(ctx context.Context, client *github.Client) (Client, error) {
	if client == nil {
		return Client{}, errors.New("client must not be nil")
	}
	return Client{
		ctx:    ctx,
		client: client,
	}, nil
}

// ResetCount resets the count of API calls.
func (c *Client) ResetCount() {
	c.count = 0
}

// GetCount retrieves the count of API calls.
func (c *Client) GetCount() int {
	return c.count
}

// ListIssuesByRepo lists the issues for the specified repository.
//
// GitHub API docs: https://developer.github.com/v3/issues/#list-issues-for-a-repository
func (c *Client) ListIssuesByRepo(owner, repo string, opt *github.IssueListByRepoOptions) ([]*github.Issue, *github.Response, error) {
	c.blockOnLimit()
	issues, resp, err := c.client.Issues.ListByRepo(c.ctx, owner, repo, opt)
	c.updateRateLimits(resp.Rate, err)
	return issues, resp, err
}

// ListIssueReactions lists the reactions for an issue.
//
// GitHub API docs: https://developer.github.com/v3/reactions/#list-reactions-for-an-issue
func (c *Client) ListIssueReactions(owner, repo string, number int, opt *github.ListOptions) ([]*github.Reaction, *github.Response, error) {
	c.blockOnLimit()
	reactions, resp, err := c.client.Reactions.ListIssueReactions(c.ctx, owner, repo, number, opt)
	c.updateRateLimits(resp.Rate, err)
	return reactions, resp, err
}

// EditIssue edits an issue.
//
// GitHub API docs: https://developer.github.com/v3/issues/#edit-an-issue
func (c *Client) EditIssue(owner, repo string, number int, req *github.IssueRequest) (*github.Issue, *github.Response, error) {
	c.blockOnLimit()
	issue, resp, err := c.client.Issues.Edit(c.ctx, owner, repo, number, req)
	c.updateRateLimits(resp.Rate, err)
	return issue, resp, err
}

// AddLabelsToIssue adds labels to an issue.
//
// GitHub API docs: https://developer.github.com/v3/issues/labels/#add-labels-to-an-issue
func (c *Client) AddLabelsToIssue(owner, repo string, number int, labels []string) ([]*github.Label, *github.Response, error) {
	c.blockOnLimit()
	labelResp, resp, err := c.client.Issues.AddLabelsToIssue(c.ctx, owner, repo, number, labels)
	c.updateRateLimits(resp.Rate, err)
	return labelResp, resp, err
}

// RemoveLabelForIssue removes a label for an issue.
//
// GitHub API docs: https://developer.github.com/v3/issues/labels/#remove-a-label-from-an-issue
func (c *Client) RemoveLabelForIssue(owner, repo string, number int, label string) (*github.Response, error) {
	c.blockOnLimit()
	resp, err := c.client.Issues.RemoveLabelForIssue(c.ctx, owner, repo, number, label)
	c.updateRateLimits(resp.Rate, err)
	return resp, err
}

// ReplaceLabelsForIssue replaces all labels for an issue.
//
// GitHub API docs: https://developer.github.com/v3/issues/labels/#replace-all-labels-for-an-issue
func (c *Client) ReplaceLabelsForIssue(owner, repo string, number int, labels []string) ([]*github.Label, *github.Response, error) {
	c.blockOnLimit()
	labelResp, resp, err := c.client.Issues.ReplaceLabelsForIssue(c.ctx, owner, repo, number, labels)
	c.updateRateLimits(resp.Rate, err)
	return labelResp, resp, err
}

// CreateIssue a new issue on the specified repository.
//
// GitHub API docs: https://developer.github.com/v3/issues/#create-an-issue
func (c *Client) CreateIssue(owner, repo string, req *github.IssueRequest) (*github.Issue, *github.Response, error) {
	c.blockOnLimit()
	issue, resp, err := c.client.Issues.Create(c.ctx, owner, repo, req)
	c.updateRateLimits(resp.Rate, err)
	return issue, resp, err
}

// CreateIssueComment creates a new comment on the specified issue.
//
// GitHub API docs: https://developer.github.com/v3/issues/comments/#create-a-comment
func (c *Client) CreateIssueComment(owner, repo string, number int, comment *github.IssueComment) (*github.IssueComment, *github.Response, error) {
	c.blockOnLimit()
	labelResp, resp, err := c.client.Issues.CreateComment(c.ctx, owner, repo, number, comment)
	c.updateRateLimits(resp.Rate, err)
	return labelResp, resp, err
}

// GetArchiveLink returns an URL to download a tarball or zipball archive for a
// repository. The archiveFormat can be specified by either the github.Tarball
// or github.Zipball constant.
//
// GitHub API docs: https://developer.github.com/v3/repos/contents/#get-archive-link
func (c *Client) GetArchiveLink(owner, repo string, format github.ArchiveFormat, opt *github.RepositoryContentGetOptions, followRedirects bool) (*url.URL, *github.Response, error) {
	c.blockOnLimit()
	url, resp, err := c.client.Repositories.GetArchiveLink(c.ctx, owner, repo, format, opt, followRedirects)
	c.updateRateLimits(resp.Rate, err)
	return url, resp, err
}

// GetRepository fetches a repository.
//
// GitHub API docs: https://developer.github.com/v3/repos/#get-a-repository
func (c *Client) GetRepository(owner, repo string) (*github.Repository, *github.Response, error) {
	c.blockOnLimit()
	r, resp, err := c.client.Repositories.Get(c.ctx, owner, repo)
	c.updateRateLimits(resp.Rate, err)
	return r, resp, err
}

// GetRepositoryBranch gets the specified branch for a repository.
//
// GitHub API docs: https://developer.github.com/v3/repos/branches/#get-a-branch
func (c *Client) GetRepositoryBranch(owner, repo, branch string) (*github.Branch, *github.Response, error) {
	c.blockOnLimit()
	b, resp, err := c.client.Repositories.GetBranch(c.ctx, owner, repo, branch)
	c.updateRateLimits(resp.Rate, err)
	return b, resp, err
}

var skew time.Duration = time.Second
var minAbuseRetry time.Duration = 2 * time.Minute

func (c *Client) blockOnLimit() {
	c.count++
	if c.isLimited() {
		if c.resetAt.After(time.Now().Add(skew)) {
			log.Printf("rate limit hit; blocking until %T", c.resetAt)
			<-time.After(c.resetAt.Sub(time.Now()) + skew)
		}
		c.mut.Lock()
		defer c.mut.Unlock()
		c.limited = false
	}
}

func (c *Client) updateRateLimits(rate github.Rate, err error) {
	if _, ok := err.(*github.RateLimitError); ok || rate.Remaining <= 0 {
		c.mut.Lock()
		defer c.mut.Unlock()
		c.limited = true
		c.resetAt = rate.Reset.Time
	} else if abuse, ok := err.(*github.AbuseRateLimitError); ok {
		c.mut.Lock()
		defer c.mut.Unlock()
		c.limited = true
		c.resetAt = time.Now().Add(max(minAbuseRetry, abuse.GetRetryAfter()))
	}
}

func (c *Client) isLimited() bool {
	c.mut.Lock()
	defer c.mut.Unlock()
	return c.limited
}

func max(a, b time.Duration) time.Duration {
	if a < b {
		return b
	}
	return a
}
