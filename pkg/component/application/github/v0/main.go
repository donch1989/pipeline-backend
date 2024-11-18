//go:generate compogen readme ./config ./README.mdx --extraContents bottom=.compogen/bottom.mdx
package github

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	_ "embed"

	"github.com/google/go-github/v62/github"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/instill-ai/pipeline-backend/config"
	"github.com/instill-ai/pipeline-backend/pkg/component/base"
	"github.com/instill-ai/pipeline-backend/pkg/data"
	"github.com/instill-ai/x/errmsg"
)

const (
	taskListPRs             = "TASK_LIST_PULL_REQUESTS"
	taskGetPR               = "TASK_GET_PULL_REQUEST"
	taskGetCommit           = "TASK_GET_COMMIT"
	taskGetReviewComments   = "TASK_LIST_REVIEW_COMMENTS"
	taskCreateReviewComment = "TASK_CREATE_REVIEW_COMMENT"
	taskListIssues          = "TASK_LIST_ISSUES"
	taskGetIssue            = "TASK_GET_ISSUE"
	taskCreateIssue         = "TASK_CREATE_ISSUE"
	taskCreateWebhook       = "TASK_CREATE_WEBHOOK"
)

var (
	//go:embed config/definition.json
	definitionJSON []byte
	//go:embed config/setup.json
	setupJSON []byte
	//go:embed config/tasks.json
	tasksJSON []byte
	//go:embed config/events.json
	eventsJSON []byte

	once sync.Once
	comp *component
)

type component struct {
	base.Component
	base.OAuthConnector
}

type execution struct {
	base.ComponentExecution
	execute func(context.Context, *structpb.Struct) (*structpb.Struct, error)
	client  Client
}

// Init returns an implementation of IComponent that interacts with Slack.
func Init(bc base.Component) *component {
	once.Do(func() {
		comp = &component{Component: bc}
		err := comp.LoadDefinition(definitionJSON, setupJSON, tasksJSON, eventsJSON, nil)
		if err != nil {
			panic(err)
		}
	})

	return comp
}

// CreateExecution initializes a component executor that can be used in a
// pipeline trigger.
func (c *component) CreateExecution(x base.ComponentExecution) (base.IExecution, error) {
	ctx := context.Background()
	githubClient := newClient(ctx, x.Setup)
	e := &execution{
		ComponentExecution: x,
		client:             githubClient,
	}
	switch x.Task {
	case taskListPRs:
		e.execute = e.client.listPullRequestsTask
	case taskGetPR:
		e.execute = e.client.getPullRequestTask
	case taskGetReviewComments:
		e.execute = e.client.listReviewCommentsTask
	case taskCreateReviewComment:
		e.execute = e.client.createReviewCommentTask
	case taskGetCommit:
		e.execute = e.client.getCommitTask
	case taskListIssues:
		e.execute = e.client.listIssuesTask
	case taskGetIssue:
		e.execute = e.client.getIssueTask
	case taskCreateIssue:
		e.execute = e.client.createIssueTask
	case taskCreateWebhook:
		e.execute = e.client.createWebhookTask
	default:
		return nil, errmsg.AddMessage(
			fmt.Errorf("not supported task: %s", x.Task),
			fmt.Sprintf("%s task is not supported.", x.Task),
		)
	}

	return e, nil
}

func (e *execution) Execute(ctx context.Context, jobs []*base.Job) error {
	for _, job := range jobs {
		input, err := job.Input.Read(ctx)
		if err != nil {
			job.Error.Error(ctx, err)
			continue
		}

		// TODO: use FillInDefaultValues for all components
		if _, err := e.FillInDefaultValues(input); err != nil {
			job.Error.Error(ctx, err)
			continue
		}

		output, err := e.execute(ctx, input)
		if err != nil {
			job.Error.Error(ctx, err)
			continue
		}

		err = job.Output.Write(ctx, output)
		if err != nil {
			job.Error.Error(ctx, err)
			continue
		}
	}

	return nil
}

func (c *component) IdentifyEvent(ctx context.Context, rawEvent *base.RawEvent) (identifierResult *base.IdentifierResult, err error) {

	if len(rawEvent.Header["x-github-event"]) > 0 && rawEvent.Header["x-github-event"][0] == "ping" {
		return &base.IdentifierResult{
			SkipTrigger: true,
			Response:    data.Map{},
		}, nil
	}
	if len(rawEvent.Header["x-github-hook-id"]) > 0 {
		hookID := rawEvent.Header["x-github-hook-id"][0]
		hookIDInt, err := strconv.Atoi(hookID)
		if err != nil {
			return nil, err
		}
		return &base.IdentifierResult{
			Identifiers: []base.Identifier{{"hook-id": hookIDInt}},
		}, nil
	}
	return nil, nil
}

func (c *component) ParseEvent(ctx context.Context, rawEvent *base.RawEvent) (parsedEvent *base.ParsedEvent, err error) {

	unmarshaler := data.NewUnmarshaler(c.BinaryFetcher)
	rawGithubEvent := rawGithubEvent{}
	err = unmarshaler.Unmarshal(ctx, rawEvent.Message, &rawGithubEvent)
	if err != nil {
		return nil, err
	}

	event := rawEvent.Header["x-github-event"][0]

	switch event + "." + rawGithubEvent.Action {
	case "star.created":
		cfg := rawGithubStarCreated{}
		err = unmarshaler.Unmarshal(ctx, rawEvent.Message, &cfg)
		if err != nil {
			return nil, err
		}
		return &base.ParsedEvent{
			ParsedMessage: rawEvent.Message,
			Response:      data.Map{},
		}, nil
	}
	return nil, fmt.Errorf("not supported event: %s.%s", event, rawGithubEvent.Action)
}

func (c *component) RegisterEvent(ctx context.Context, settings *base.RegisterEventSettings) ([]base.Identifier, error) {

	// TODO: support more events
	setup, err := settings.Setup.ToStructValue()
	if err != nil {
		return nil, err
	}
	githubClient := newClient(ctx, setup.GetStructValue())

	unmarshaler := data.NewUnmarshaler(c.BinaryFetcher)
	cfg := githubEventStarCreatedConfig{}
	err = unmarshaler.Unmarshal(ctx, settings.Config, &cfg)
	if err != nil {
		return nil, err
	}
	namespace, repo, ok := strings.Cut(cfg.Repository, "/")
	if !ok {
		return nil, fmt.Errorf("invalid repository format: %s", cfg.Repository)
	}

	// TODO: avoid directly reading from config
	host := config.Config.Server.InstillCoreHost
	url := fmt.Sprintf("%s/v1beta/pipeline-webhooks/github", host)

	// TODO: add secret
	hooks, _, err := githubClient.Repositories.ListHooks(ctx, namespace, repo, &github.ListOptions{})
	if err != nil {
		return nil, err
	}

	existingHook := false
	hookID := int64(0)
	for _, hook := range hooks {

		if *hook.Config.URL == url && hook.Events[0] == "star" {
			existingHook = true
			_, _, err := githubClient.Repositories.EditHook(ctx, namespace, repo, hook.GetID(), &github.Hook{
				Active: github.Bool(true),
			})
			if err != nil {
				return nil, err
			}
			hookID = hook.GetID()
			break
		}
	}

	if !existingHook {
		insecureSSL := github.String("1")
		if strings.HasPrefix(host, "https://") {
			insecureSSL = github.String("0")
		}
		hook, _, err := githubClient.Repositories.CreateHook(ctx, namespace, repo, &github.Hook{
			Config: &github.HookConfig{
				URL:         github.String(url),
				ContentType: github.String("json"),
				InsecureSSL: insecureSSL,
			},
			Events: []string{"star"},
			Active: github.Bool(true),
		})

		if err != nil {
			return nil, err
		}
		hookID = hook.GetID()
	}

	return []base.Identifier{{"hook-id": hookID}}, nil
}

func (c *component) UnregisterEvent(ctx context.Context, settings *base.UnregisterEventSettings, identifier []base.Identifier) error {
	// return nil
	setup, err := settings.Setup.ToStructValue()
	if err != nil {
		return err
	}
	githubClient := newClient(ctx, setup.GetStructValue())

	unmarshaler := data.NewUnmarshaler(c.BinaryFetcher)
	cfg := githubEventStarCreatedConfig{}
	err = unmarshaler.Unmarshal(ctx, settings.Config, &cfg)
	if err != nil {
		return err
	}
	namespace, repo, ok := strings.Cut(cfg.Repository, "/")
	if !ok {
		return fmt.Errorf("invalid repository format: %s", cfg.Repository)
	}

	for _, id := range identifier {
		if hookID, ok := id["hook-id"]; ok {
			_, _, err := githubClient.Repositories.EditHook(ctx, namespace, repo, int64(hookID.(float64)), &github.Hook{
				Active: github.Bool(false),
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// SupportsOAuth checks whether the component is configured to support OAuth.
func (c *component) SupportsOAuth() bool {
	return c.OAuthConnector.SupportsOAuth()
}
