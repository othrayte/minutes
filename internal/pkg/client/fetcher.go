package client

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/gabor-boros/minutes/internal/pkg/utils"
	"github.com/gabor-boros/minutes/internal/pkg/worklog"
)

const (
	// DefaultPageSize used by paginated fetchers setting the fetched page size.
	// The minimum page sizes can be different per client, but the 50 items per
	// page is usually supported everywhere.
	DefaultPageSize int = 50
	// DefaultPageSizeParam used by paginated fetchers setting page size parameter.
	DefaultPageSizeParam string = "per_page"
	// DefaultPageParam used by paginated fetchers setting the page parameter.
	DefaultPageParam string = "page"

	// Options for handling tasks when an item is assigned to multiple tasks
	// Split the entry into multiple entries, one for each task with the time split between them
	SplitAcrossTasks string = "split"
	// Only use the first task found in the entry, prefering the tasks from the summary, then tags, then project
	// Note: the order between the tags is not guaranteed
	FirstTaskOnly string = "first-only"
)

var MultipleTaskModes = []string{
	SplitAcrossTasks,
	FirstTaskOnly,
}

var (
	// ErrFetchEntries wraps the error when fetch failed.
	ErrFetchEntries = errors.New("failed to fetch entries")
)

type TaskExtractionOpts struct {
	// TagsAsTasksRegex sets the regular expression used for extracting tasks
	// from the list of tags.
	TagsAsTasksRegex   *regexp.Regexp
	TaskInSummaryRegex *regexp.Regexp
	TaskInProjectRegex *regexp.Regexp

	MultipleTaskMode string
}

// FetchOpts specifies the only options for Fetchers.
// In contract to the BaseClientOpts, these options shall not be extended or
// overridden.
type FetchOpts struct {
	User  string
	Start time.Time
	End   time.Time

	TaskExtraction TaskExtractionOpts
}

// Fetcher specifies the functions used to fetch worklog entries.
type Fetcher interface {
	// FetchEntries from a given source and return the list of worklog entries
	// If the fetching resulted in an error, the list of worklog entries will be
	// nil and an error will return.
	FetchEntries(ctx context.Context, opts *FetchOpts) (worklog.Entries, error)
}

type PaginatedFetchResponse struct {
	EntriesPerPage int
	TotalEntries   int
}

type PaginatedFetchFunc = func(context.Context, string) (interface{}, *PaginatedFetchResponse, error)
type PaginatedParseFunc = func(interface{}, *FetchOpts) (worklog.Entries, error)

type PaginatedFetchOpts struct {
	BaseFetchOpts *FetchOpts

	URL           string
	PageSize      int
	PageSizeParam string
	PageParam     string

	FetchFunc PaginatedFetchFunc
	ParseFunc PaginatedParseFunc
}

func ExtractTasks(e *worklog.Entry, tags []worklog.IDNameField, opts *TaskExtractionOpts) []worklog.IDNameField {
	var tasks []worklog.IDNameField
	if utils.IsRegexSet(opts.TaskInSummaryRegex) {
		tasks = append(tasks, e.TasksFromSummary(opts.TaskInSummaryRegex)...)
	}

	if utils.IsRegexSet(opts.TagsAsTasksRegex) && (opts.MultipleTaskMode == SplitAcrossTasks || len(tasks) == 0) {
		tasks = append(tasks, e.TasksFromTags(tags, opts.TagsAsTasksRegex)...)
	}

	if utils.IsRegexSet(opts.TaskInProjectRegex) && (opts.MultipleTaskMode == SplitAcrossTasks || len(tasks) == 0) {
		tasks = append(tasks, e.TasksFromProject(opts.TaskInProjectRegex)...)
	}

	if opts.MultipleTaskMode == FirstTaskOnly && len(tasks) > 1 {
		tasks = tasks[:1]
	}

	return tasks
}
