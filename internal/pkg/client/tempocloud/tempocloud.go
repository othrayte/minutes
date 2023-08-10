package tempocloud

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/gabor-boros/minutes/internal/pkg/client"
	"github.com/gabor-boros/minutes/internal/pkg/utils"
	"github.com/gabor-boros/minutes/internal/pkg/worklog"
)

const (
	// TempoPathWorklogCreate is the endpoint used to create new worklogs.
	TempoPathWorklogCreate string = "/4/worklogs"

	// JiraPathIssue is the endpoint used to query the jira issue id
	JiraPathIssue string = "/rest/api/3/issue/" //<ISSUE-KEY>
)

// Issue represents the Jira issue the time logged against.
type Issue struct {
	ID         int    `json:"id"`
	Key        string `json:"key"`
	AccountKey string `json:"accountKey"`
	ProjectID  int    `json:"projectId"`
	ProjectKey string `json:"projectKey"`
	Summary    string `json:"summary"`
}

// JiraIssue represents the Jira issue data returned when querying Jira
type JiraIssue struct {
	ID  int    `json:"id,string"`
	Key string `json:"key"`
}

// UploadEntry represents the payload to create a new worklog in Tempo.
// StartDate must be in the YYYY-MM-DD format, required by Tempo.
// StartTime must be in the HH:MM:SS format, required by Tempo.
type UploadEntry struct {
	Comment          string `json:"description,omitempty"`
	IssueID          int    `json:"issueId,omitempty"`
	StartDate        string `json:"startDate,omitempty"`
	StartTime        string `json:"startTime,omitempty"`
	BillableSeconds  int    `json:"billableSeconds,omitempty"`
	TimeSpentSeconds int    `json:"timeSpentSeconds,omitempty"`
	AuthorAccountID  string `json:"authorAccountId,omitempty"`
}

// ClientOpts is the client specific options, extending client.BaseClientOpts.
type ClientOpts struct {
	client.BaseClientOpts
	TempoAuth    client.TokenAuth
	JiraAuth     client.BasicAuth
	TempoBaseURL string
	JiraBaseURL  string
}

type tempoClient struct {
	*client.BaseClientOpts
	tempoHttpClient *client.HTTPClient
	jiraHttpClient  *client.HTTPClient
	*client.DefaultUploader
	tempoAuthenticator client.Authenticator
	jiraAuthenticator  client.Authenticator
}

func (c *tempoClient) UploadEntries(ctx context.Context, entries worklog.Entries, errChan chan error, opts *client.UploadOpts) {
	createURL, err := c.tempoHttpClient.URL(TempoPathWorklogCreate, map[string]string{})
	if err != nil {
		errChan <- fmt.Errorf("%v: %v", client.ErrUploadEntries, err)
		return
	}
	getIssueURL, err := c.jiraHttpClient.URL(JiraPathIssue, map[string]string{})
	if err != nil {
		errChan <- fmt.Errorf("%v: %v", client.ErrUploadEntries, err)
		return
	}

	for _, groupEntries := range entries.GroupByTask() {
		go func(ctx context.Context, entries worklog.Entries, errChan chan error, opts *client.UploadOpts) {
			for _, entry := range entries {
				tracker := c.StartTracking(entry, opts.ProgressWriter)

				issueKey := entry.Task.Name
				resp, err := c.jiraHttpClient.Call(ctx, &client.HTTPRequestOpts{
					Method:  http.MethodGet,
					Url:     getIssueURL + issueKey,
					Auth:    c.jiraAuthenticator,
					Timeout: c.Timeout,
					Headers: map[string]string{
						"Content-Type": "application/json",
					},
				})

				if err != nil {
					err = fmt.Errorf("%v: %v: %v", client.ErrUploadEntries, issueKey, err)
					c.StopTracking(tracker, err)
					errChan <- err
					continue
				}

				var issue JiraIssue
				if err = json.Unmarshal(resp, &issue); err != nil {
					err = fmt.Errorf("%v: %v", client.ErrFetchEntries, err)
				}

				if err != nil {
					c.StopTracking(tracker, err)
					errChan <- err
					continue
				}

				billableDuration := entry.BillableDuration
				unbillableDuration := entry.UnbillableDuration
				totalTimeSpent := billableDuration + unbillableDuration

				if opts.TreatDurationAsBilled {
					billableDuration = entry.UnbillableDuration + entry.BillableDuration
					unbillableDuration = 0
				}

				if opts.RoundToClosestMinute {
					billableDuration = time.Second * time.Duration(math.Round(billableDuration.Minutes())*60)
					unbillableDuration = time.Second * time.Duration(math.Round(unbillableDuration.Minutes())*60)
					totalTimeSpent = billableDuration + unbillableDuration
				}

				uploadEntry := &UploadEntry{
					Comment:          entry.Summary,
					IssueID:          issue.ID,
					StartDate:        utils.DateFormatISO8601.Format(entry.Start.Local()),
					StartTime:        entry.Start.Local().Format("15:04:05"),
					BillableSeconds:  int(billableDuration.Seconds()),
					TimeSpentSeconds: int(totalTimeSpent.Seconds()),
					AuthorAccountID:  opts.User,
				}

				_, err = c.tempoHttpClient.Call(ctx, &client.HTTPRequestOpts{
					Method:  http.MethodPost,
					Url:     createURL,
					Auth:    c.tempoAuthenticator,
					Timeout: c.Timeout,
					Data:    uploadEntry,
					Headers: map[string]string{
						"Content-Type": "application/json",
					},
				})

				if err != nil {
					err = fmt.Errorf("%v: %+v: %v", client.ErrUploadEntries, uploadEntry, err)
				}

				c.StopTracking(tracker, err)
				errChan <- err
			}
		}(ctx, groupEntries, errChan, opts)
	}
}

func newClient(opts *ClientOpts) (*tempoClient, error) {
	tempoBaseURL, err := url.Parse(opts.TempoBaseURL)
	if err != nil {
		return nil, err
	}

	jiraBaseURL, err := url.Parse(opts.JiraBaseURL)
	if err != nil {
		return nil, err
	}

	tempoAuthenticator, err := client.NewTokenAuth(opts.TempoAuth.Header, opts.TempoAuth.TokenName, opts.TempoAuth.Token)
	if err != nil {
		return nil, err
	}

	jiraAuthenticator, err := client.NewBasicAuth(opts.JiraAuth.Username, opts.JiraAuth.Password)
	if err != nil {
		return nil, err
	}

	return &tempoClient{
		tempoAuthenticator: tempoAuthenticator,
		jiraAuthenticator:  jiraAuthenticator,
		tempoHttpClient:    &client.HTTPClient{BaseURL: tempoBaseURL},
		jiraHttpClient:     &client.HTTPClient{BaseURL: jiraBaseURL},
		BaseClientOpts:     &opts.BaseClientOpts,
	}, nil
}

// NewUploader returns a new Tempo client for uploading entries.
func NewUploader(opts *ClientOpts) (client.Uploader, error) {
	return newClient(opts)
}
