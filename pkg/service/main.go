package service

import (
	"context"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"github.com/gofrs/uuid"
	"github.com/redis/go-redis/v9"
	"go.einride.tech/aip/filtering"
	"go.einride.tech/aip/ordering"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/instill-ai/pipeline-backend/pkg/acl"
	"github.com/instill-ai/pipeline-backend/pkg/external"
	"github.com/instill-ai/pipeline-backend/pkg/logger"
	"github.com/instill-ai/pipeline-backend/pkg/memory"
	"github.com/instill-ai/pipeline-backend/pkg/repository"
	"github.com/instill-ai/pipeline-backend/pkg/resource"

	componentstore "github.com/instill-ai/pipeline-backend/pkg/component/store"
	mgmtpb "github.com/instill-ai/protogen-go/core/mgmt/v1beta"
	pb "github.com/instill-ai/protogen-go/vdp/pipeline/v1beta"
	miniox "github.com/instill-ai/x/minio"
)

// Service interface
type Service interface {
	GetHubStats(ctx context.Context) (*pb.GetHubStatsResponse, error)
	ListPipelines(ctx context.Context, pageSize int32, pageToken string, view pb.Pipeline_View, visibility *pb.Pipeline_Visibility, filter filtering.Filter, showDeleted bool, order ordering.OrderBy) ([]*pb.Pipeline, int32, string, error)
	GetPipelineByUID(ctx context.Context, uid uuid.UUID, view pb.Pipeline_View) (*pb.Pipeline, error)
	CreateNamespacePipeline(ctx context.Context, ns resource.Namespace, pipeline *pb.Pipeline) (*pb.Pipeline, error)
	ListNamespacePipelines(ctx context.Context, ns resource.Namespace, pageSize int32, pageToken string, view pb.Pipeline_View, visibility *pb.Pipeline_Visibility, filter filtering.Filter, showDeleted bool, order ordering.OrderBy) ([]*pb.Pipeline, int32, string, error)
	GetNamespacePipelineByID(ctx context.Context, ns resource.Namespace, id string, view pb.Pipeline_View) (*pb.Pipeline, error)
	UpdateNamespacePipelineByID(ctx context.Context, ns resource.Namespace, id string, updatedPipeline *pb.Pipeline) (*pb.Pipeline, error)
	UpdateNamespacePipelineIDByID(ctx context.Context, ns resource.Namespace, id string, newID string) (*pb.Pipeline, error)
	DeleteNamespacePipelineByID(ctx context.Context, ns resource.Namespace, id string) error
	ValidateNamespacePipelineByID(ctx context.Context, ns resource.Namespace, id string) ([]*pb.ErrPipelineValidation, error)
	GetNamespacePipelineLatestReleaseUID(ctx context.Context, ns resource.Namespace, id string) (uuid.UUID, error)
	CloneNamespacePipeline(ctx context.Context, ns resource.Namespace, id, targetNamespaceID, targetPipelineID, description string, sharing *pb.Sharing) (*pb.Pipeline, error)

	ListPipelinesAdmin(ctx context.Context, pageSize int32, pageToken string, view pb.Pipeline_View, filter filtering.Filter, showDeleted bool) ([]*pb.Pipeline, int32, string, error)
	GetPipelineByUIDAdmin(ctx context.Context, uid uuid.UUID, view pb.Pipeline_View) (*pb.Pipeline, error)

	CreateNamespacePipelineRelease(ctx context.Context, ns resource.Namespace, pipelineUID uuid.UUID, pipelineRelease *pb.PipelineRelease) (*pb.PipelineRelease, error)
	ListNamespacePipelineReleases(ctx context.Context, ns resource.Namespace, pipelineUID uuid.UUID, pageSize int32, pageToken string, view pb.Pipeline_View, filter filtering.Filter, showDeleted bool) ([]*pb.PipelineRelease, int32, string, error)
	GetNamespacePipelineReleaseByID(ctx context.Context, ns resource.Namespace, pipelineUID uuid.UUID, id string, view pb.Pipeline_View) (*pb.PipelineRelease, error)
	UpdateNamespacePipelineReleaseByID(ctx context.Context, ns resource.Namespace, pipelineUID uuid.UUID, id string, updatedPipelineRelease *pb.PipelineRelease) (*pb.PipelineRelease, error)
	DeleteNamespacePipelineReleaseByID(ctx context.Context, ns resource.Namespace, pipelineUID uuid.UUID, id string) error
	RestoreNamespacePipelineReleaseByID(ctx context.Context, ns resource.Namespace, pipelineUID uuid.UUID, id string) error
	UpdateNamespacePipelineReleaseIDByID(ctx context.Context, ns resource.Namespace, pipelineUID uuid.UUID, id string, newID string) (*pb.PipelineRelease, error)
	CloneNamespacePipelineRelease(ctx context.Context, ns resource.Namespace, pipelineUID uuid.UUID, id, targetNamespaceID, targetPipelineID, description string, sharing *pb.Sharing) (*pb.Pipeline, error)

	CreateNamespaceSecret(ctx context.Context, ns resource.Namespace, secret *pb.Secret) (*pb.Secret, error)
	ListNamespaceSecrets(ctx context.Context, ns resource.Namespace, pageSize int32, pageToken string, filter filtering.Filter) ([]*pb.Secret, int32, string, error)
	GetNamespaceSecretByID(ctx context.Context, ns resource.Namespace, id string) (*pb.Secret, error)
	UpdateNamespaceSecretByID(ctx context.Context, ns resource.Namespace, id string, updatedSecret *pb.Secret) (*pb.Secret, error)
	DeleteNamespaceSecretByID(ctx context.Context, ns resource.Namespace, id string) error

	TriggerNamespacePipelineByID(ctx context.Context, ns resource.Namespace, id string, data []*pb.TriggerData, pipelineTriggerID string, returnTraces bool) ([]*structpb.Struct, *pb.TriggerMetadata, error)
	TriggerAsyncNamespacePipelineByID(ctx context.Context, ns resource.Namespace, id string, data []*pb.TriggerData, pipelineTriggerID string, returnTraces bool) (*longrunningpb.Operation, error)

	CheckPipelineEventCode(ctx context.Context, ns resource.Namespace, id string, code string) (bool, error)
	// HandleNamespacePipelineEventByID(ctx context.Context, ns resource.Namespace, id string, eventID string, data *structpb.Struct, pipelineTriggerID string) (*structpb.Struct, error)
	DispatchPipelineWebhookEvent(ctx context.Context, params DispatchPipelineWebhookEventParams) (DispatchPipelineWebhookEventResult, error)

	TriggerNamespacePipelineReleaseByID(ctx context.Context, ns resource.Namespace, pipelineUID uuid.UUID, id string, data []*pb.TriggerData, pipelineTriggerID string, returnTraces bool) ([]*structpb.Struct, *pb.TriggerMetadata, error)
	TriggerAsyncNamespacePipelineReleaseByID(ctx context.Context, ns resource.Namespace, pipelineUID uuid.UUID, id string, data []*pb.TriggerData, pipelineTriggerID string, returnTraces bool) (*longrunningpb.Operation, error)
	GetOperation(ctx context.Context, workflowID string) (*longrunningpb.Operation, error)

	GetCtxUserNamespace(ctx context.Context) (resource.Namespace, error)
	GetNamespaceByID(ctx context.Context, namespaceID string) (resource.Namespace, error)
	GetNamespaceByUID(ctx context.Context, namespaceUID uuid.UUID) (resource.Namespace, error)

	ListComponentDefinitions(context.Context, *pb.ListComponentDefinitionsRequest) (*pb.ListComponentDefinitionsResponse, error)

	ListPipelineRuns(ctx context.Context, req *pb.ListPipelineRunsRequest, filter filtering.Filter) (*pb.ListPipelineRunsResponse, error)
	ListComponentRuns(ctx context.Context, req *pb.ListComponentRunsRequest, filter filtering.Filter) (*pb.ListComponentRunsResponse, error)
	ListPipelineRunsByRequester(ctx context.Context, req *pb.ListPipelineRunsByRequesterRequest) (*pb.ListPipelineRunsByRequesterResponse, error)

	GetIntegration(_ context.Context, id string, _ pb.View) (*pb.Integration, error)
	ListIntegrations(context.Context, *pb.ListIntegrationsRequest) (*pb.ListIntegrationsResponse, error)
	CreateNamespaceConnection(context.Context, *pb.CreateNamespaceConnectionRequest) (*pb.Connection, error)
	UpdateNamespaceConnection(context.Context, *pb.UpdateNamespaceConnectionRequest) (*pb.Connection, error)
	DeleteNamespaceConnection(_ context.Context, namespaceID, id string) error
	GetNamespaceConnection(context.Context, *pb.GetNamespaceConnectionRequest) (*pb.Connection, error)
	ListNamespaceConnections(context.Context, *pb.ListNamespaceConnectionsRequest) (*pb.ListNamespaceConnectionsResponse, error)
	ListPipelineIDsByConnectionID(context.Context, *pb.ListPipelineIDsByConnectionIDRequest) (*pb.ListPipelineIDsByConnectionIDResponse, error)
}

// TriggerResult defines a new type to encapsulate the stream data
type TriggerResult struct {
	Struct   []*structpb.Struct
	Metadata *pb.TriggerMetadata
}

// Now, we don't need the artifact service client in the service layer.
// However, we keep it here for now because we may need it in the future.
// service is the implementation of the Service interface
type service struct {
	repository               repository.Repository
	redisClient              *redis.Client
	temporalClient           client.Client
	component                *componentstore.Store
	mgmtPrivateServiceClient mgmtpb.MgmtPrivateServiceClient
	aclClient                acl.ACLClientInterface
	converter                Converter
	minioClient              miniox.MinioI
	memory                   memory.MemoryStore
	log                      *zap.Logger
	workerUID                uuid.UUID
	retentionHandler         MetadataRetentionHandler
	binaryFetcher            external.BinaryFetcher
}

// ServiceConfig is the configuration for the service
type ServiceConfig struct {
	Repository               repository.Repository
	RedisClient              *redis.Client
	TemporalClient           client.Client
	ACLClient                acl.ACLClientInterface
	Converter                Converter
	MgmtPrivateServiceClient mgmtpb.MgmtPrivateServiceClient
	MinioClient              miniox.MinioI
	ComponentStore           *componentstore.Store
	Memory                   memory.MemoryStore
	WorkerUID                uuid.UUID
	RetentionHandler         MetadataRetentionHandler
	BinaryFetcher            external.BinaryFetcher
}

// NewService initiates a service instance
func NewService(
	cfg ServiceConfig,
) Service {
	zapLogger, _ := logger.GetZapLogger(context.Background())
	if cfg.RetentionHandler == nil {
		cfg.RetentionHandler = NewRetentionHandler()
	}

	return &service{
		repository:               cfg.Repository,
		redisClient:              cfg.RedisClient,
		temporalClient:           cfg.TemporalClient,
		mgmtPrivateServiceClient: cfg.MgmtPrivateServiceClient,
		component:                cfg.ComponentStore,
		aclClient:                cfg.ACLClient,
		converter:                cfg.Converter,
		minioClient:              cfg.MinioClient,
		memory:                   cfg.Memory,
		log:                      zapLogger,
		workerUID:                cfg.WorkerUID,
		retentionHandler:         cfg.RetentionHandler,
		binaryFetcher:            cfg.BinaryFetcher,
	}
}
