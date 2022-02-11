package rpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	database "github.com/instill-ai/pipeline-backend/internal/db"
	metadataUtil "github.com/instill-ai/pipeline-backend/internal/grpc/metadata"
	structUtil "github.com/instill-ai/pipeline-backend/internal/struct/util"
	"github.com/instill-ai/pipeline-backend/pkg/model"
	"github.com/instill-ai/pipeline-backend/pkg/repository"
	"github.com/instill-ai/pipeline-backend/pkg/service"
	pb "github.com/instill-ai/protogen-go/pipeline"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

func getUsername(ctx context.Context) (string, error) {
	if metadatas, ok := metadataUtil.ExtractFromMetadata(ctx, "Username"); ok {
		if len(metadatas) == 0 {
			return "", status.Error(codes.FailedPrecondition, "Username not found in your request")
		}
		return metadatas[0], nil
	} else {
		return "", status.Error(codes.FailedPrecondition, "Error when extract metadata")
	}
}

type pipelineServiceHandlers struct {
	pipelineService service.PipelineService
}

func NewPipelineServiceHandlers(pipelineService service.PipelineService) pb.PipelineServer {
	return &pipelineServiceHandlers{
		pipelineService: pipelineService,
	}
}

func (s *pipelineServiceHandlers) Liveness(ctx context.Context, in *emptypb.Empty) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{Status: "ok", Code: pb.HealthCheckResponse_SERVING}, nil
}

func (s *pipelineServiceHandlers) Readiness(ctx context.Context, in *emptypb.Empty) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{Status: "ok", Code: pb.HealthCheckResponse_SERVING}, nil
}

func (s *pipelineServiceHandlers) CreatePipeline(ctx context.Context, in *pb.CreatePipelineRequest) (*pb.PipelineInfo, error) {

	username, err := getUsername(ctx)
	if err != nil {
		return &pb.PipelineInfo{}, err
	}

	// Covert to model
	entity := model.Pipeline{
		Name:        in.Name,
		Description: in.Description,
		Recipe:      unmarshalRecipe(in.Recipe),
		Active:      in.Active,
		Namespace:   username,
	}

	pipeline, err := s.pipelineService.CreatePipeline(entity)
	if err != nil {
		return nil, err
	}

	// We need to manually set the custom header to have a StatusCreated http response for REST endpoint
	grpc.SetHeader(ctx, metadata.Pairs("x-http-code", strconv.Itoa(http.StatusCreated)))

	return marshalPipeline(&pipeline), nil
}

func (s *pipelineServiceHandlers) ListPipelines(ctx context.Context, in *pb.ListPipelinesRequest) (*pb.ListPipelinesResponse, error) {

	username, err := getUsername(ctx)
	if err != nil {
		return &pb.ListPipelinesResponse{}, err
	}

	pipelines, err := s.pipelineService.ListPipelines(model.ListPipelineQuery{
		Namespace:  username,
		WithRecipe: in.WithRecipe,
	})
	if err != nil {
		return nil, err
	}

	var resp pb.ListPipelinesResponse

	for _, pipeline := range pipelines {
		resp.Contents = append(resp.Contents, marshalPipeline(&pipeline))
	}

	return &resp, nil
}

func (s *pipelineServiceHandlers) GetPipeline(ctx context.Context, in *pb.GetPipelineRequest) (*pb.PipelineInfo, error) {

	username, err := getUsername(ctx)
	if err != nil {
		return &pb.PipelineInfo{}, err
	}

	pipeline, err := s.pipelineService.GetPipelineByName(username, in.Name)
	if err != nil {
		return nil, err
	}

	return marshalPipeline(&pipeline), nil
}

func (s *pipelineServiceHandlers) UpdatePipeline(ctx context.Context, in *pb.UpdatePipelineRequest) (*pb.PipelineInfo, error) {

	username, err := getUsername(ctx)
	if err != nil {
		return &pb.PipelineInfo{}, err
	}

	// Covert to model
	entity := model.Pipeline{
		Name:      in.Pipeline.Name,
		Namespace: username,
	}
	if in.UpdateMask != nil && len(in.UpdateMask.Paths) > 0 {
		entity.UpdatedAt = time.Now()

		for _, field := range in.UpdateMask.Paths {
			switch field {
			case "description":
				entity.Description = in.Pipeline.Description
			case "active":
				entity.Active = in.Pipeline.Active
			}
			if strings.Contains(field, "recipe") {
				entity.Recipe = unmarshalRecipe(in.Pipeline.Recipe)
			}
		}
	}

	pipeline, err := s.pipelineService.UpdatePipeline(entity)
	if err != nil {
		return nil, err
	}

	return marshalPipeline(&pipeline), nil
}

func (s *pipelineServiceHandlers) DeletePipeline(ctx context.Context, in *pb.DeletePipelineRequest) (*emptypb.Empty, error) {

	username, err := getUsername(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	if err := s.pipelineService.DeletePipeline(username, in.Name); err != nil {
		return nil, err
	}

	// We need to manually set the custom header to have a StatusCreated http response for REST endpoint
	grpc.SetHeader(ctx, metadata.Pairs("x-http-code", strconv.Itoa(http.StatusNoContent)))

	return &emptypb.Empty{}, nil
}

func (s *pipelineServiceHandlers) TriggerPipeline(ctx context.Context, in *pb.TriggerPipelineRequest) (*structpb.Struct, error) {

	username, err := getUsername(ctx)
	if err != nil {
		return &structpb.Struct{}, err
	}

	pipeline, err := s.pipelineService.GetPipelineByName(username, in.Name)
	if err != nil {
		return &structpb.Struct{}, err
	}

	if err := s.pipelineService.ValidateTriggerPipeline(username, in.Name, pipeline); err != nil {
		return &structpb.Struct{}, err
	}

	if obj, err := s.pipelineService.TriggerPipeline(username, *in, pipeline); err != nil {
		return &structpb.Struct{}, err
	} else {
		ret, err := structUtil.StructToProtobufStruct(obj)
		if err != nil {
			return &structpb.Struct{}, status.Error(codes.Internal, err.Error())
		}
		return ret, nil
	}
}

func (s *pipelineServiceHandlers) TriggerPipelineByUpload(stream pb.Pipeline_TriggerPipelineByUploadServer) error {

	username, err := getUsername(stream.Context())
	if err != nil {
		return err
	}

	data, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.Unknown, "cannot receive trigger info")
	}

	pipeline, err := s.pipelineService.GetPipelineByName(username, data.Name)
	if err != nil {
		return err
	}

	if err := s.pipelineService.ValidateTriggerPipeline(username, data.Name, pipeline); err != nil {
		return err
	}

	// Read chuck
	buf := bytes.Buffer{}
	for {
		data, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}

			return status.Errorf(codes.Internal, "failed unexpectadely while reading chunks from stream: %s", err.Error())
		}

		if data.Contents == nil {
			continue
		}

		if len(data.Contents) > 1 {
			return status.Error(codes.InvalidArgument, "only accept upload single file")
		}

		if _, err := buf.Write(data.Contents[0].Chunk); err != nil {
			return status.Errorf(codes.Internal, "failed unexpectadely while reading chunks from stream: %s", err.Error())
		}
	}

	var obj interface{}
	if obj, err = s.pipelineService.TriggerPipelineByUpload(username, buf, pipeline); err != nil {
		return err
	}

	ret, err := structUtil.StructToProtobufStruct(obj)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	stream.SendAndClose(ret)

	return nil
}

func HandleUploadOutput(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {

	contentType := r.Header.Get("Content-Type")

	if strings.Contains(contentType, "multipart/form-data") {
		username := r.Header.Get("Username")
		pipelineName := pathParams["name"]

		if username == "" {
			w.Header().Add("Content-Type", "application/json+problem")
			w.WriteHeader(422)
			obj, _ := json.Marshal(model.Error{
				Status: 422,
				Title:  "Required parameter missing",
				Detail: "Required parameter Jwt-Sub not found in your header",
			})
			w.Write(obj)
		}
		if pipelineName == "" {
			w.Header().Add("Content-Type", "application/json+problem")
			w.WriteHeader(422)
			obj, _ := json.Marshal(model.Error{
				Status: 422,
				Title:  "Required parameter missing",
				Detail: "Required parameter pipeline id not found in your path",
			})
			w.Write(obj)
		}

		db := database.GetConnection()
		pipelineRepository := repository.NewPipelineRepository(db)
		pipelineService := service.NewPipelineService(pipelineRepository)

		pipeline, err := pipelineService.GetPipelineByName(username, pipelineName)
		if err != nil {
			w.Header().Add("Content-Type", "application/json+problem")
			w.WriteHeader(400)
			obj, _ := json.Marshal(model.Error{
				Status: 400,
				Title:  "Required parameter missing",
				Detail: "Required parameter pipeline id not found in your path",
			})
			w.Write(obj)
		}

		r.ParseMultipartForm(4 << 20)
		file, _, err := r.FormFile("contents")
		if err != nil {
			w.Header().Add("Content-Type", "application/json+problem")
			w.WriteHeader(400)
			obj, _ := json.Marshal(model.Error{
				Status: 500,
				Title:  "Internal Error",
				Detail: "Error while reading file from request",
			})
			w.Write(obj)
		}
		defer file.Close()

		reader := bufio.NewReader(file)
		buf := bytes.NewBuffer(make([]byte, 0))
		part := make([]byte, 1024)

		count := 0
		for {
			if count, err = reader.Read(part); err != nil {
				break
			}
			buf.Write(part[:count])
		}
		if err != io.EOF {
			w.Header().Add("Content-Type", "application/json+problem")
			w.WriteHeader(400)
			obj, _ := json.Marshal(model.Error{
				Status: 400,
				Title:  "Error Reading",
			})
			w.Write(obj)
		}

		var obj interface{}
		if obj, err = pipelineService.TriggerPipelineByUpload(username, *buf, pipeline); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Add("Content-Type", "application/json+problem")
		w.WriteHeader(200)
		ret, _ := json.Marshal(obj)
		w.Write(ret)
	} else {
		w.Header().Add("Content-Type", "application/json+problem")
		w.WriteHeader(405)
	}
}