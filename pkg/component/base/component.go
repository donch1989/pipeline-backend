package base

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofrs/uuid"
	"github.com/lestrrat-go/jsref/provider"
	"go.uber.org/zap"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	jsoniter "github.com/json-iterator/go"

	"github.com/instill-ai/pipeline-backend/pkg/component/internal/jsonref"
	"github.com/instill-ai/pipeline-backend/pkg/data/format"
	"github.com/instill-ai/pipeline-backend/pkg/external"

	pb "github.com/instill-ai/protogen-go/vdp/pipeline/v1beta"
)

const conditionJSON = `
{
	"type": "string",
	"instillUIOrder": 1,
	"instillShortDescription": "config whether the component will be executed or skipped",
	"instillAcceptFormats": ["string"],
    "instillUpstreamTypes": ["value", "template"]
}
`

// InstillExtension is the extension for the component.
type InstillExtension struct {
	jsoniter.DummyExtension
}

// UpdateStructDescriptor updates the struct descriptor for the component.
func (e *InstillExtension) UpdateStructDescriptor(structDescriptor *jsoniter.StructDescriptor) {

	// We use kebab-case for JSON keys in component input and output, while
	// vendors sometimes use camelCase or snake_case in requests and responses.
	// This often requires us to write two structs and convert between them.
	// However, most of the time, they can share the same struct with only
	// different tags. `jsoniter` is a good tool that can help us manage using
	// different tags to encode/decode JSON. Here, we implement an extension
	// where the JSON encoder/decoder will first use the `instill` tag as the
	// JSON key; if no `instill` tag is present, it will use the default `json`
	// tag. We'll use jsoniter in `ConvertFromStructpb()` and
	// `ConvertToStructpb()`.

	for _, v := range structDescriptor.Fields {
		t := v.Field.Tag()
		if instill, ok := t.Lookup("instill"); ok {
			v.FromNames = []string{instill}
			v.ToNames = []string{instill}
		}
	}
}

func init() {
	jsoniter.RegisterExtension(&InstillExtension{})
}

// IComponent is the interface that wraps the basic component methods.
// All component need to implement this interface.
type IComponent interface {
	GetDefinitionID() string
	GetDefinitionUID() uuid.UUID
	GetLogger() *zap.Logger
	GetTaskInputSchemas() map[string]string
	GetTaskOutputSchemas() map[string]string

	LoadDefinition(definitionJSON, setupJSON, tasksJSON []byte, eventJSONBytes []byte, additionalJSONBytes map[string][]byte) error

	// Note: Some content in the definition JSON schema needs to be generated
	// by sysVars or component setting.
	GetDefinition(sysVars map[string]any, compConfig *ComponentConfig) (*pb.ComponentDefinition, error)

	// CreateExecution takes a ComponentExecution that can be used to compose
	// the core component behaviour with the particular business logic in the
	// implementation.
	CreateExecution(base ComponentExecution) (IExecution, error)
	Test(sysVars map[string]any, config *structpb.Struct) error

	IsSecretField(target string) bool
	SupportsOAuth() bool

	// Note: These functions are for the pipeline run-on-event feature,
	// which is still experimental and may change at any time.

	// RegisterEvent registers an event handler for the component. It performs
	// two main tasks:
	// 1. Registers a webhook URL with the vendor service if required
	// 2. Generates a identifier for the event registration that will be used to
	//    route incoming events to the correct pipeline
	//
	// The identifier returned by this method will be stored in backend and used
	// later to match incoming webhook events with their corresponding pipeline.
	RegisterEvent(ctx context.Context, settings *RegisterEventSettings) (identifier []Identifier, err error)

	// UnregisterEvent unregisters an event handler for the component.
	UnregisterEvent(ctx context.Context, settings *UnregisterEventSettings, identifier []Identifier) error

	// IdentifyEvent identifies the event and returns the identifiers.
	IdentifyEvent(ctx context.Context, rawEvent *RawEvent) (identifierResult *IdentifierResult, err error)

	// ParseEvent parses the raw event and returns a parsed event.
	// The parsed event contains:
	// - parsed message: the processed event data
	// - webhook response: any response that should be sent back to the webhook caller
	ParseEvent(ctx context.Context, rawEvent *RawEvent) (parsedEvent *ParsedEvent, err error)

	UsageHandlerCreator() UsageHandlerCreator
}

// Identifier is the identifier for the event.
type Identifier map[string]any

// EventSettings is the settings for the event.
type EventSettings struct {
	// TODO: The Config field represents the component configuration settings
	// while Setup contains initialization parameters. Consider renaming to
	// a more explicit name for clarity.
	Config format.Value
	Setup  format.Value
}

// RegisterEventSettings is the settings for registering an event.
type RegisterEventSettings struct {
	EventSettings
	RegistrationUID uuid.UUID
}

// UnregisterEventSettings is the settings for unregistering an event.
type UnregisterEventSettings struct {
	EventSettings
}

// RawEvent is the raw event from the webhook.
type RawEvent struct {
	EventSettings
	Header  map[string][]string
	Message format.Value
}

// ParsedEvent is the parsed event from the raw event.
type ParsedEvent struct {
	ParsedMessage format.Value
	Response      format.Value
}

// IdentifierResult is the result of identifying an event.
type IdentifierResult struct {
	SkipTrigger bool
	Identifiers []Identifier
	Response    format.Value
}

// Component implements the common component methods.
type Component struct {
	Logger          *zap.Logger
	NewUsageHandler UsageHandlerCreator

	taskInputSchemas  map[string]string
	taskOutputSchemas map[string]string

	definition               *pb.ComponentDefinition
	secretFields             []string
	inputAcceptFormatsFields map[string]map[string][]string
	outputFormatsFields      map[string]map[string]string

	BinaryFetcher external.BinaryFetcher
}

// IdentifyEvent is not implemented for the base component.
func (c *Component) IdentifyEvent(ctx context.Context, rawEvent *RawEvent) (identifierResult *IdentifierResult, err error) {
	return nil, fmt.Errorf("not implemented")
}

// ParseEvent is not implemented for the base component.
func (c *Component) ParseEvent(ctx context.Context, rawEvent *RawEvent) (*ParsedEvent, error) {
	return nil, fmt.Errorf("not implemented")
}

// RegisterEvent is not implemented for the base component.
func (c *Component) RegisterEvent(context.Context, *RegisterEventSettings) ([]Identifier, error) {
	return nil, fmt.Errorf("not implemented")
}

// UnregisterEvent is not implemented for the base component.
func (c *Component) UnregisterEvent(context.Context, *UnregisterEventSettings, []Identifier) error {
	return fmt.Errorf("not implemented")
}

func convertDataSpecToCompSpec(dataSpec *structpb.Struct) (*structpb.Struct, error) {
	// var err error
	compSpec := proto.Clone(dataSpec).(*structpb.Struct)
	if _, ok := compSpec.Fields["const"]; ok {
		return compSpec, nil
	}

	isFreeform := checkFreeForm(compSpec)

	// Check if we have anyOf/allOf/oneOf before requiring type field
	hasCompositeSchema := false
	for _, field := range []string{"anyOf", "allOf", "oneOf"} {
		if _, ok := compSpec.Fields[field]; ok {
			hasCompositeSchema = true
			break
		}
	}

	if _, ok := compSpec.Fields["type"]; !ok && !isFreeform && !hasCompositeSchema {
		return nil, fmt.Errorf("type missing: %+v", compSpec)
	}

	// Handle composite schemas (anyOf/allOf/oneOf)
	for _, target := range []string{"allOf", "anyOf", "oneOf"} {
		if fields, ok := compSpec.Fields[target]; ok {
			for idx, schema := range fields.GetListValue().GetValues() {
				converted, err := convertDataSpecToCompSpec(schema.GetStructValue())
				if err != nil {
					return nil, err
				}
				fields.GetListValue().Values[idx] = structpb.NewStructValue(converted)
			}
		}
	}

	if compSpec.Fields["type"] != nil && compSpec.Fields["type"].GetStringValue() == "object" {
		// Always add required field for object type if missing
		if _, ok := compSpec.Fields["required"]; !ok {
			compSpec.Fields["required"] = structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{}})
		}

		if _, ok := compSpec.Fields["instillEditOnNodeFields"]; !ok {
			compSpec.Fields["instillEditOnNodeFields"] = compSpec.Fields["required"]
		}

		if _, ok := compSpec.Fields["properties"]; ok {
			for k, v := range compSpec.Fields["properties"].GetStructValue().AsMap() {
				switch val := v.(type) {
				case map[string]any:
					s, err := structpb.NewStruct(val)
					if err != nil {
						return nil, err
					}
					converted, err := convertDataSpecToCompSpec(s)
					if err != nil {
						return nil, err
					}
					compSpec.Fields["properties"].GetStructValue().Fields[k] = structpb.NewStructValue(converted)
				case []interface{}:
					listValue := &structpb.ListValue{
						Values: make([]*structpb.Value, len(val)),
					}
					for i, item := range val {
						value, err := structpb.NewValue(item)
						if err != nil {
							return nil, err
						}
						listValue.Values[i] = value
					}
					compSpec.Fields["properties"].GetStructValue().Fields[k] = structpb.NewListValue(listValue)
				case string, bool, float64, int64:
					value, err := structpb.NewValue(val)
					if err != nil {
						return nil, err
					}
					compSpec.Fields["properties"].GetStructValue().Fields[k] = value
				default:
					return nil, fmt.Errorf("unsupported type: %T", v)
				}
			}
		}
		if _, ok := compSpec.Fields["patternProperties"]; ok {
			for k, v := range compSpec.Fields["patternProperties"].GetStructValue().AsMap() {
				switch val := v.(type) {
				case map[string]any:
					s, err := structpb.NewStruct(val)
					if err != nil {
						return nil, err
					}
					converted, err := convertDataSpecToCompSpec(s)
					if err != nil {
						return nil, err
					}
					compSpec.Fields["patternProperties"].GetStructValue().Fields[k] = structpb.NewStructValue(converted)
				case string, bool, float64, int64:
					value, err := structpb.NewValue(val)
					if err != nil {
						return nil, err
					}
					compSpec.Fields["patternProperties"].GetStructValue().Fields[k] = value
				default:
					return nil, fmt.Errorf("unsupported type: %T", v)
				}
			}
		}
		for _, target := range []string{"allOf", "anyOf", "oneOf"} {
			if _, ok := compSpec.Fields[target]; ok {
				for idx, item := range compSpec.Fields[target].GetListValue().AsSlice() {
					s, err := structpb.NewStruct(item.(map[string]any))
					if err != nil {
						return nil, err
					}
					converted, err := convertDataSpecToCompSpec(s)
					if err != nil {
						return nil, err
					}
					compSpec.Fields[target].GetListValue().Values[idx] = structpb.NewStructValue(converted)
				}
			}
		}

	} else {
		if _, ok := compSpec.Fields["instillUIOrder"]; !ok {
			compSpec.Fields["instillUIOrder"] = structpb.NewNumberValue(0)
		}
		original := proto.Clone(compSpec).(*structpb.Struct)
		delete(original.Fields, "title")
		delete(original.Fields, "description")
		delete(original.Fields, "instillShortDescription")
		delete(original.Fields, "instillAcceptFormats")
		delete(original.Fields, "instillUIOrder")
		delete(original.Fields, "instillUpstreamTypes")

		newCompSpec := &structpb.Struct{Fields: make(map[string]*structpb.Value)}

		newCompSpec.Fields["title"] = structpb.NewStringValue(compSpec.Fields["title"].GetStringValue())
		newCompSpec.Fields["description"] = structpb.NewStringValue(compSpec.Fields["description"].GetStringValue())
		if _, ok := compSpec.Fields["instillShortDescription"]; ok {
			newCompSpec.Fields["instillShortDescription"] = compSpec.Fields["instillShortDescription"]
		} else {
			newCompSpec.Fields["instillShortDescription"] = newCompSpec.Fields["description"]
		}
		newCompSpec.Fields["instillUIOrder"] = structpb.NewNumberValue(compSpec.Fields["instillUIOrder"].GetNumberValue())
		if compSpec.Fields["instillAcceptFormats"] != nil {
			newCompSpec.Fields["instillAcceptFormats"] = structpb.NewListValue(compSpec.Fields["instillAcceptFormats"].GetListValue())
		}
		newCompSpec.Fields["instillUpstreamTypes"] = structpb.NewListValue(compSpec.Fields["instillUpstreamTypes"].GetListValue())
		newCompSpec.Fields["anyOf"] = structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{}})

		for _, v := range compSpec.Fields["instillUpstreamTypes"].GetListValue().GetValues() {
			if v.GetStringValue() == "value" {
				original.Fields["instillUpstreamType"] = v
				newCompSpec.Fields["anyOf"].GetListValue().Values = append(newCompSpec.Fields["anyOf"].GetListValue().Values, structpb.NewStructValue(original))
			}
			if v.GetStringValue() == "reference" {
				item, err := structpb.NewValue(
					map[string]any{
						"type":                "string",
						"pattern":             "^\\{.*\\}$",
						"instillUpstreamType": "reference",
					},
				)
				if err != nil {
					return nil, err
				}
				newCompSpec.Fields["anyOf"].GetListValue().Values = append(newCompSpec.Fields["anyOf"].GetListValue().Values, item)
			}
			if v.GetStringValue() == "template" {
				item, err := structpb.NewValue(
					map[string]any{
						"type":                "string",
						"instillUpstreamType": "template",
					},
				)
				if err != nil {
					return nil, err
				}
				newCompSpec.Fields["anyOf"].GetListValue().Values = append(newCompSpec.Fields["anyOf"].GetListValue().Values, item)
			}

		}

		compSpec = newCompSpec

	}
	return compSpec, nil
}

const taskPrefix = "TASK_"

// TaskIDToTitle builds a Task title from its ID. This is used when the `title`
// key in the task definition isn't present.
func TaskIDToTitle(id string) string {
	title := strings.ReplaceAll(id, taskPrefix, "")
	title = strings.ReplaceAll(title, "_", " ")
	return cases.Title(language.English).String(title)
}

func generateComponentTaskCards(tasks []string, taskStructs map[string]*structpb.Struct) []*pb.ComponentTask {
	taskCards := make([]*pb.ComponentTask, 0, len(tasks))
	for _, k := range tasks {
		if v, ok := taskStructs[k]; ok {
			title := v.Fields["title"].GetStringValue()
			if title == "" {
				title = TaskIDToTitle(k)
			}

			description := taskStructs[k].Fields["instillShortDescription"].GetStringValue()

			taskCards = append(taskCards, &pb.ComponentTask{
				Name:        k,
				Title:       title,
				Description: description,
			})
		}
	}

	return taskCards
}

func generateComponentSpec(title string, tasks []*pb.ComponentTask, taskStructs map[string]*structpb.Struct) (*structpb.Struct, error) {
	var err error
	componentSpec := &structpb.Struct{Fields: map[string]*structpb.Value{}}
	componentSpec.Fields["$schema"] = structpb.NewStringValue("http://json-schema.org/draft-07/schema#")
	componentSpec.Fields["title"] = structpb.NewStringValue(fmt.Sprintf("%s Component", title))
	componentSpec.Fields["type"] = structpb.NewStringValue("object")

	oneOfList := &structpb.ListValue{
		Values: []*structpb.Value{},
	}
	for _, task := range tasks {
		taskName := task.Name

		oneOf := &structpb.Struct{Fields: map[string]*structpb.Value{}}
		oneOf.Fields["type"] = structpb.NewStringValue("object")
		oneOf.Fields["properties"] = structpb.NewStructValue(&structpb.Struct{Fields: make(map[string]*structpb.Value)})

		oneOf.Fields["properties"].GetStructValue().Fields["task"], err = structpb.NewValue(map[string]any{
			"const": task.Name,
			"title": task.Title,
		})
		if err != nil {
			return nil, err
		}

		if taskStructs[taskName].Fields["description"].GetStringValue() != "" {
			oneOf.Fields["properties"].GetStructValue().Fields["task"].GetStructValue().Fields["description"] = structpb.NewStringValue(taskStructs[taskName].Fields["description"].GetStringValue())
		}

		if task.Description != "" {
			oneOf.Fields["properties"].GetStructValue().Fields["task"].GetStructValue().Fields["instillShortDescription"] = structpb.NewStringValue(task.Description)
		}
		taskJSONStruct := proto.Clone(taskStructs[taskName]).(*structpb.Struct).Fields["input"].GetStructValue()

		compInputStruct, err := convertDataSpecToCompSpec(taskJSONStruct)
		if err != nil {
			return nil, fmt.Errorf("task %s: %s error: %+v", title, task, err)
		}

		condition := &structpb.Struct{}
		err = protojson.Unmarshal([]byte(conditionJSON), condition)
		if err != nil {
			panic(err)
		}
		oneOf.Fields["properties"].GetStructValue().Fields["condition"] = structpb.NewStructValue(condition)
		oneOf.Fields["properties"].GetStructValue().Fields["input"] = structpb.NewStructValue(compInputStruct)
		if taskStructs[taskName].Fields["metadata"] != nil {
			metadataStruct := proto.Clone(taskStructs[taskName]).(*structpb.Struct).Fields["metadata"].GetStructValue()
			oneOf.Fields["properties"].GetStructValue().Fields["metadata"] = structpb.NewStructValue(metadataStruct)
		}

		// oneOf
		oneOfList.Values = append(oneOfList.Values, structpb.NewStructValue(oneOf))
	}

	componentSpec.Fields["oneOf"] = structpb.NewListValue(oneOfList)

	if err != nil {
		return nil, err
	}

	return componentSpec, nil
}

func formatDataSpec(dataSpec *structpb.Struct) (*structpb.Struct, error) {
	compSpec := proto.Clone(dataSpec).(*structpb.Struct)
	if compSpec == nil {
		return compSpec, nil
	}
	if compSpec.Fields == nil {
		compSpec.Fields = make(map[string]*structpb.Value)
		return compSpec, nil
	}
	if _, ok := compSpec.Fields["const"]; ok {
		return compSpec, nil
	}

	isFreeform := checkFreeForm(compSpec)

	if _, ok := compSpec.Fields["type"]; !ok && !isFreeform {
		return nil, fmt.Errorf("type missing: %+v", compSpec)
	} else if compSpec.Fields["type"].GetStringValue() == "array" {
		if _, ok := compSpec.Fields["instillUIOrder"]; !ok {
			compSpec.Fields["instillUIOrder"] = structpb.NewNumberValue(0)
		}

		if items, ok := compSpec.Fields["items"]; ok {
			switch items.Kind.(type) {
			case *structpb.Value_StructValue:
				converted, err := formatDataSpec(items.GetStructValue())
				if err != nil {
					return nil, err
				}
				compSpec.Fields["items"] = structpb.NewStructValue(converted)
			case *structpb.Value_ListValue:
				compSpec.Fields["items"] = items
			default:
				return nil, fmt.Errorf("unsupported type for items: %T", items.Kind)
			}
		}
	} else if compSpec.Fields["type"].GetStringValue() == "object" {
		if _, ok := compSpec.Fields["instillUIOrder"]; !ok {
			compSpec.Fields["instillUIOrder"] = structpb.NewNumberValue(0)
		}
		if _, ok := compSpec.Fields["required"]; !ok {
			compSpec.Fields["required"] = structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{}})
		}
		if _, ok := compSpec.Fields["instillEditOnNodeFields"]; !ok {
			compSpec.Fields["instillEditOnNodeFields"] = compSpec.Fields["required"]
		}

		if _, ok := compSpec.Fields["properties"]; ok {
			for k, v := range compSpec.Fields["properties"].GetStructValue().Fields {
				switch v.Kind.(type) {
				case *structpb.Value_StructValue:
					converted, err := formatDataSpec(v.GetStructValue())
					if err != nil {
						return nil, err
					}
					compSpec.Fields["properties"].GetStructValue().Fields[k] = structpb.NewStructValue(converted)
				case *structpb.Value_ListValue:
					// Keep list values as they are
					compSpec.Fields["properties"].GetStructValue().Fields[k] = v
				case *structpb.Value_StringValue, *structpb.Value_NumberValue, *structpb.Value_BoolValue:
					// Keep primitive values as they are
					compSpec.Fields["properties"].GetStructValue().Fields[k] = v
				}
			}
		}
		if _, ok := compSpec.Fields["patternProperties"]; ok {
			for k, v := range compSpec.Fields["patternProperties"].GetStructValue().AsMap() {
				s, err := structpb.NewStruct(v.(map[string]any))
				if err != nil {
					return nil, err
				}
				converted, err := formatDataSpec(s)
				if err != nil {
					return nil, err
				}
				compSpec.Fields["patternProperties"].GetStructValue().Fields[k] = structpb.NewStructValue(converted)

			}
		}
		for _, target := range []string{"allOf", "anyOf", "oneOf"} {
			if _, ok := compSpec.Fields[target]; ok {
				for idx, item := range compSpec.Fields[target].GetListValue().AsSlice() {
					s, err := structpb.NewStruct(item.(map[string]any))
					if err != nil {
						return nil, err
					}
					converted, err := formatDataSpec(s)
					if err != nil {
						return nil, err
					}
					compSpec.Fields[target].GetListValue().AsSlice()[idx] = structpb.NewStructValue(converted)
				}
			}
		}

	} else {
		if _, ok := compSpec.Fields["instillUIOrder"]; !ok {
			compSpec.Fields["instillUIOrder"] = structpb.NewNumberValue(0)
		}

		newCompSpec := &structpb.Struct{Fields: make(map[string]*structpb.Value)}

		newCompSpec.Fields["type"] = structpb.NewStringValue(compSpec.Fields["type"].GetStringValue())
		newCompSpec.Fields["title"] = structpb.NewStringValue(compSpec.Fields["title"].GetStringValue())
		newCompSpec.Fields["description"] = structpb.NewStringValue(compSpec.Fields["description"].GetStringValue())
		if _, ok := newCompSpec.Fields["instillShortDescription"]; ok {
			newCompSpec.Fields["instillShortDescription"] = compSpec.Fields["instillShortDescription"]
		} else {
			newCompSpec.Fields["instillShortDescription"] = newCompSpec.Fields["description"]
		}
		newCompSpec.Fields["instillUIOrder"] = structpb.NewNumberValue(compSpec.Fields["instillUIOrder"].GetNumberValue())
		if compSpec.Fields["instillFormat"] != nil {
			newCompSpec.Fields["instillFormat"] = structpb.NewStringValue(compSpec.Fields["instillFormat"].GetStringValue())
		}

		compSpec = newCompSpec

	}
	return compSpec, nil
}

// EventJSON is the JSON for the event.
type EventJSON map[string]Event

// Event is the event for the component.
type Event struct {
	Title           string `json:"title"`
	Description     string `json:"description"`
	ConfigSchema    any    `json:"configSchema"`
	MessageSchema   any    `json:"messageSchema"`
	MessageExamples []any  `json:"messageExamples"`
}

func generateEventSpecs(eventJSONBytes []byte) (map[string]*pb.EventSpecification, error) {

	specs := map[string]*pb.EventSpecification{}
	var j EventJSON
	err := json.Unmarshal(eventJSONBytes, &j)
	if err != nil {
		return nil, err
	}
	for t, e := range j {
		c, err := json.Marshal(e.ConfigSchema)
		if err != nil {
			return nil, err
		}
		pbConfigSchema := &structpb.Struct{}
		err = protojson.Unmarshal(c, pbConfigSchema)
		if err != nil {
			return nil, err
		}

		m, err := json.Marshal(e.MessageSchema)
		if err != nil {
			return nil, err
		}
		pbMessageSchema := &structpb.Struct{}
		err = protojson.Unmarshal(m, pbMessageSchema)
		if err != nil {
			return nil, err
		}
		pbMessageExamples := make([]*structpb.Struct, 0, len(e.MessageExamples))
		for _, ex := range e.MessageExamples {
			pbMessageExample := &structpb.Struct{}
			exs, err := json.Marshal(ex)
			if err != nil {
				return nil, err
			}
			err = protojson.Unmarshal(exs, pbMessageExample)
			if err != nil {
				return nil, err
			}
			pbMessageExamples = append(pbMessageExamples, pbMessageExample)
		}
		specs[t] = &pb.EventSpecification{
			Title:           e.Title,
			Description:     e.Description,
			ConfigSchema:    pbConfigSchema,
			MessageSchema:   pbMessageSchema,
			MessageExamples: pbMessageExamples,
		}
	}
	return specs, nil
}

func generateDataSpecs(tasks map[string]*structpb.Struct) (map[string]*pb.DataSpecification, error) {

	specs := map[string]*pb.DataSpecification{}
	for k := range tasks {
		spec := &pb.DataSpecification{}
		var err error
		taskJSONStruct := proto.Clone(tasks[k]).(*structpb.Struct)
		spec.Input, err = formatDataSpec(taskJSONStruct.Fields["input"].GetStructValue())
		if err != nil {
			return nil, err
		}
		spec.Output, err = formatDataSpec(taskJSONStruct.Fields["output"].GetStructValue())
		if err != nil {
			return nil, err
		}
		specs[k] = spec
	}

	return specs, nil
}

func loadTasks(availableTasks []string, tasksJSONBytes []byte) ([]*pb.ComponentTask, map[string]*structpb.Struct, error) {

	taskStructs := map[string]*structpb.Struct{}
	var err error

	tasksJSONMap := map[string]map[string]any{}
	err = json.Unmarshal(tasksJSONBytes, &tasksJSONMap)
	if err != nil {
		return nil, nil, err
	}

	for _, t := range availableTasks {
		if v, ok := tasksJSONMap[t]; ok {
			taskStructs[t], err = structpb.NewStruct(v)
			if err != nil {
				return nil, nil, err
			}

		}
	}
	tasks := generateComponentTaskCards(availableTasks, taskStructs)
	return tasks, taskStructs, nil
}

// ConvertFromStructpb converts from structpb.Struct to a struct
func ConvertFromStructpb(from *structpb.Struct, to any) error {
	inputJSON, err := protojson.Marshal(from)
	if err != nil {
		return err
	}

	err = jsoniter.Unmarshal(inputJSON, to)
	if err != nil {
		return err
	}
	return nil
}

// ConvertToStructpb converts from a struct to structpb.Struct
func ConvertToStructpb(from any) (*structpb.Struct, error) {
	to := &structpb.Struct{}

	outputJSON, err := jsoniter.Marshal(from)
	if err != nil {
		return nil, err
	}

	err = protojson.Unmarshal(outputJSON, to)
	if err != nil {
		return nil, err
	}
	return to, nil
}

// RenderJSON renders the JSON for the component.
func RenderJSON(tasksJSONBytes []byte, additionalJSONBytes map[string][]byte) ([]byte, error) {
	var err error
	mp := provider.NewMap()
	for k, v := range additionalJSONBytes {
		var i any
		err = json.Unmarshal(v, &i)
		if err != nil {
			return nil, err
		}
		err = mp.Set(k, i)
		if err != nil {
			return nil, err
		}
	}
	res := jsonref.New()
	err = res.AddProvider(mp)
	if err != nil {
		return nil, err
	}
	err = res.AddProvider(provider.NewHTTP())
	if err != nil {
		return nil, err
	}

	var tasksJSON any
	err = json.Unmarshal(tasksJSONBytes, &tasksJSON)
	if err != nil {
		return nil, err
	}

	result, err := res.Resolve(tasksJSON, "", jsonref.WithRecursiveResolution(true))
	if err != nil {
		return nil, err
	}
	renderedTasksJSON, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return renderedTasksJSON, nil

}

// For formats such as `*`, `semi-structured/*`, and `semi-structured/json` we
// treat them as freeform data. Thus, there is no need to set the `type` in the
// JSON schema.
func checkFreeForm(compSpec *structpb.Struct) bool {
	acceptFormats := compSpec.Fields["instillAcceptFormats"].GetListValue().AsSlice()

	formats := make([]any, 0, len(acceptFormats)+1) // This avoids reallocations when appending values to the slice.
	formats = append(formats, acceptFormats...)

	if instillFormat := compSpec.Fields["instillFormat"].GetStringValue(); instillFormat != "" {
		formats = append(formats, instillFormat)
	}
	if len(formats) == 0 {
		return true
	}

	for _, v := range formats {
		if v.(string) == "*" || v.(string) == "semi-structured/*" || v.(string) == "semi-structured/json" {
			return true
		}
	}

	return false
}

// GetDefinitionID returns the component definition ID.
func (c *Component) GetDefinitionID() string {
	return c.definition.Id
}

// GetDefinitionUID returns the component definition UID.
func (c *Component) GetDefinitionUID() uuid.UUID {
	return uuid.FromStringOrNil(c.definition.Uid)
}

// GetLogger returns the component's logger. If it hasn't been initialized, a
// no-op logger is returned.
func (c *Component) GetLogger() *zap.Logger {
	if c.Logger == nil {
		return zap.NewNop()
	}

	return c.Logger
}

// GetDefinition returns the component definition.
func (c *Component) GetDefinition(sysVars map[string]any, compConfig *ComponentConfig) (*pb.ComponentDefinition, error) {
	return c.definition, nil
}

// GetTaskInputSchemas returns the task input schemas.
func (c *Component) GetTaskInputSchemas() map[string]string {
	return c.taskInputSchemas
}

// GetTaskOutputSchemas returns the task output schemas.
func (c *Component) GetTaskOutputSchemas() map[string]string {
	return c.taskOutputSchemas
}

// LoadDefinition loads the component definition, setup, tasks, events and additional JSON files.
// The definition files are currently loaded together but could be refactored to load separately.
func (c *Component) LoadDefinition(definitionJSONBytes, setupJSONBytes, tasksJSONBytes []byte, eventsJSONBytes []byte, additionalJSONBytes map[string][]byte) error {
	var err error
	var definitionJSON any

	c.secretFields = []string{}

	err = json.Unmarshal(definitionJSONBytes, &definitionJSON)
	if err != nil {
		return err
	}
	renderedTasksJSON, err := RenderJSON(tasksJSONBytes, additionalJSONBytes)
	if err != nil {
		return err
	}

	availableTasks := []string{}
	for _, availableTask := range definitionJSON.(map[string]any)["availableTasks"].([]any) {
		availableTasks = append(availableTasks, availableTask.(string))
	}

	tasks, taskStructs, err := loadTasks(availableTasks, renderedTasksJSON)
	if err != nil {
		return err
	}

	c.taskInputSchemas = map[string]string{}
	c.taskOutputSchemas = map[string]string{}
	for k := range taskStructs {
		var s []byte
		s, err = protojson.Marshal(taskStructs[k].Fields["input"].GetStructValue())
		if err != nil {
			return err
		}
		c.taskInputSchemas[k] = string(s)

		s, err = protojson.Marshal(taskStructs[k].Fields["output"].GetStructValue())
		if err != nil {
			return err
		}
		c.taskOutputSchemas[k] = string(s)
	}

	c.definition = &pb.ComponentDefinition{}
	err = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(definitionJSONBytes, c.definition)
	if err != nil {
		return err
	}

	c.definition.Name = fmt.Sprintf("component-definitions/%s", c.definition.Id)
	c.definition.Tasks = tasks
	if c.definition.Spec == nil {
		c.definition.Spec = &pb.ComponentDefinition_Spec{}
	}
	c.definition.Spec.ComponentSpecification, err = generateComponentSpec(c.definition.Title, tasks, taskStructs)
	if err != nil {
		return err
	}

	raw := &structpb.Struct{}
	err = protojson.Unmarshal(definitionJSONBytes, raw)
	if err != nil {
		return err
	}

	// TODO: Avoid using structpb traversal here.
	if setupJSONBytes != nil {
		setup := &structpb.Struct{}
		err = protojson.Unmarshal(setupJSONBytes, setup)
		if err != nil {
			return err
		}
		setup, err := c.refineResourceSpec(setup)
		if err != nil {
			return err
		}
		configPropStruct := &structpb.Struct{Fields: map[string]*structpb.Value{}}
		configPropStruct.Fields["setup"] = structpb.NewStructValue(setup)
		c.definition.Spec.ComponentSpecification.Fields["properties"] = structpb.NewStructValue(configPropStruct)

	}

	if eventsJSONBytes != nil {
		c.definition.Spec.EventSpecifications, err = generateEventSpecs(eventsJSONBytes)
		if err != nil {
			return err
		}
	}

	c.definition.Spec.DataSpecifications, err = generateDataSpecs(taskStructs)
	if err != nil {
		return err
	}

	c.initSecretField(c.definition)
	c.initInputAcceptFormatsFields()
	c.initOutputFormatsFields()

	return nil

}

func (c *Component) refineResourceSpec(resourceSpec *structpb.Struct) (*structpb.Struct, error) {

	spec := proto.Clone(resourceSpec).(*structpb.Struct)
	if _, ok := spec.Fields["instillShortDescription"]; !ok {
		spec.Fields["instillShortDescription"] = structpb.NewStringValue(spec.Fields["description"].GetStringValue())
	}

	if _, ok := spec.Fields["properties"]; ok {
		for k, v := range spec.Fields["properties"].GetStructValue().AsMap() {
			switch val := v.(type) {
			case map[string]any:
				s, err := structpb.NewStruct(val)
				if err != nil {
					return nil, err
				}
				converted, err := c.refineResourceSpec(s)
				if err != nil {
					return nil, err
				}
				spec.Fields["properties"].GetStructValue().Fields[k] = structpb.NewStructValue(converted)
			case string, bool, float64, int64:
				// Handle primitive types directly
				value, err := structpb.NewValue(val)
				if err != nil {
					return nil, err
				}
				spec.Fields["properties"].GetStructValue().Fields[k] = value
			default:
				return nil, fmt.Errorf("unsupported type: %T", v)
			}
		}
	}
	if _, ok := spec.Fields["patternProperties"]; ok {
		for k, v := range spec.Fields["patternProperties"].GetStructValue().AsMap() {
			switch val := v.(type) {
			case map[string]any:
				s, err := structpb.NewStruct(val)
				if err != nil {
					return nil, err
				}
				converted, err := c.refineResourceSpec(s)
				if err != nil {
					return nil, err
				}
				spec.Fields["patternProperties"].GetStructValue().Fields[k] = structpb.NewStructValue(converted)
			case string, bool, float64, int64:
				// Handle primitive types directly
				value, err := structpb.NewValue(val)
				if err != nil {
					return nil, err
				}
				spec.Fields["patternProperties"].GetStructValue().Fields[k] = value
			default:
				return nil, fmt.Errorf("unsupported type: %T", v)
			}
		}
	}
	for _, target := range []string{"allOf", "anyOf", "oneOf"} {
		if _, ok := spec.Fields[target]; ok {
			for idx, item := range spec.Fields[target].GetListValue().AsSlice() {
				switch val := item.(type) {
				case map[string]any:
					s, err := structpb.NewStruct(val)
					if err != nil {
						return nil, err
					}
					converted, err := c.refineResourceSpec(s)
					if err != nil {
						return nil, err
					}
					spec.Fields[target].GetListValue().AsSlice()[idx] = structpb.NewStructValue(converted)
				case string, bool, float64, int64:
					// Handle primitive types directly
					value, err := structpb.NewValue(val)
					if err != nil {
						return nil, err
					}
					spec.Fields[target].GetListValue().AsSlice()[idx] = value
				default:
					return nil, fmt.Errorf("unsupported type: %T", item)
				}
			}
		}
	}

	return spec, nil
}

// IsSecretField checks if the target field is secret field
func (c *Component) IsSecretField(target string) bool {
	for _, field := range c.secretFields {
		if target == field {
			return true
		}
	}
	return false
}

// ListSecretFields lists the secret fields by definition id
func (c *Component) ListSecretFields() ([]string, error) {
	return c.secretFields, nil
}

func (c *Component) initSecretField(def *pb.ComponentDefinition) {
	if c.secretFields == nil {
		c.secretFields = []string{}
	}
	secretFields := []string{}
	setup := def.Spec.GetComponentSpecification().GetFields()["properties"].GetStructValue().GetFields()["setup"].GetStructValue()
	secretFields = c.traverseSecretField(setup.GetFields()["properties"], "", secretFields)
	if l, ok := setup.GetFields()["oneOf"]; ok {
		for _, v := range l.GetListValue().Values {
			secretFields = c.traverseSecretField(v.GetStructValue().GetFields()["properties"], "", secretFields)
		}
	}
	c.secretFields = secretFields
}

func (c *Component) traverseSecretField(input *structpb.Value, prefix string, secretFields []string) []string {
	for key, v := range input.GetStructValue().GetFields() {
		if isSecret, ok := v.GetStructValue().GetFields()["instillSecret"]; ok {
			if isSecret.GetBoolValue() || isSecret.GetStringValue() == "true" {
				secretFields = append(secretFields, fmt.Sprintf("%s%s", prefix, key))
			}
		}
		if tp, ok := v.GetStructValue().GetFields()["type"]; ok {
			if tp.GetStringValue() == "object" {
				if l, ok := v.GetStructValue().GetFields()["oneOf"]; ok {
					for _, v := range l.GetListValue().Values {
						secretFields = c.traverseSecretField(v.GetStructValue().GetFields()["properties"], fmt.Sprintf("%s%s.", prefix, key), secretFields)
					}
				}
				secretFields = c.traverseSecretField(v.GetStructValue().GetFields()["properties"], fmt.Sprintf("%s%s.", prefix, key), secretFields)
			}

		}
	}

	return secretFields
}

// SupportsOAuth is false by default. To support OAuth, component
// implementations must be composed with `OAuthComponent`.
func (c *Component) SupportsOAuth() bool {
	return false
}

// ListInputAcceptFormatsFields returns the input accept formats fields.
func (c *Component) ListInputAcceptFormatsFields() (map[string]map[string][]string, error) {
	return c.inputAcceptFormatsFields, nil
}

func (c *Component) initInputAcceptFormatsFields() {
	inputAcceptFormatsFields := map[string]map[string][]string{}

	for task, sch := range c.GetTaskInputSchemas() {
		inputAcceptFormatsFields[task] = map[string][]string{}
		input := &structpb.Struct{}
		_ = protojson.Unmarshal([]byte(sch), input)
		inputAcceptFormatsFields[task] = c.traverseInputAcceptFormatsFields(input.GetFields()["properties"], "", inputAcceptFormatsFields[task])
		if l, ok := input.GetFields()["oneOf"]; ok {
			for _, v := range l.GetListValue().Values {
				inputAcceptFormatsFields[task] = c.traverseInputAcceptFormatsFields(v.GetStructValue().GetFields()["properties"], "", inputAcceptFormatsFields[task])
			}
		}
		c.inputAcceptFormatsFields = inputAcceptFormatsFields
	}

}

func (c *Component) traverseInputAcceptFormatsFields(input *structpb.Value, prefix string, inputAcceptFormatsFields map[string][]string) map[string][]string {
	for key, v := range input.GetStructValue().GetFields() {

		if v, ok := v.GetStructValue().GetFields()["instillAcceptFormats"]; ok {
			for _, f := range v.GetListValue().Values {
				k := fmt.Sprintf("%s%s", prefix, key)
				inputAcceptFormatsFields[k] = append(inputAcceptFormatsFields[k], f.GetStringValue())
			}
		}
		if tp, ok := v.GetStructValue().GetFields()["type"]; ok {
			if tp.GetStringValue() == "object" {
				if l, ok := v.GetStructValue().GetFields()["oneOf"]; ok {
					for _, v := range l.GetListValue().Values {
						inputAcceptFormatsFields = c.traverseInputAcceptFormatsFields(v.GetStructValue().GetFields()["properties"], fmt.Sprintf("%s%s.", prefix, key), inputAcceptFormatsFields)
					}
				}
				inputAcceptFormatsFields = c.traverseInputAcceptFormatsFields(v.GetStructValue().GetFields()["properties"], fmt.Sprintf("%s%s.", prefix, key), inputAcceptFormatsFields)
			}

		}
	}

	return inputAcceptFormatsFields
}

// ListOutputFormatsFields returns the output formats fields.
func (c *Component) ListOutputFormatsFields() (map[string]map[string]string, error) {
	return c.outputFormatsFields, nil
}

func (c *Component) initOutputFormatsFields() {
	outputFormatsFields := map[string]map[string]string{}

	for task, sch := range c.GetTaskOutputSchemas() {
		outputFormatsFields[task] = map[string]string{}
		output := &structpb.Struct{}
		_ = protojson.Unmarshal([]byte(sch), output)
		outputFormatsFields[task] = c.traverseOutputFormatsFields(output.GetFields()["properties"], "", outputFormatsFields[task])
		if l, ok := output.GetFields()["oneOf"]; ok {
			for _, v := range l.GetListValue().Values {
				outputFormatsFields[task] = c.traverseOutputFormatsFields(v.GetStructValue().GetFields()["properties"], "", outputFormatsFields[task])
			}
		}
		c.outputFormatsFields = outputFormatsFields

	}

}

func (c *Component) traverseOutputFormatsFields(input *structpb.Value, prefix string, outputFormatsFields map[string]string) map[string]string {
	for key, v := range input.GetStructValue().GetFields() {

		if v, ok := v.GetStructValue().GetFields()["instillFormat"]; ok {
			k := fmt.Sprintf("%s%s", prefix, key)
			outputFormatsFields[k] = v.GetStringValue()
		}
		if tp, ok := v.GetStructValue().GetFields()["type"]; ok {
			if tp.GetStringValue() == "object" {
				if l, ok := v.GetStructValue().GetFields()["oneOf"]; ok {
					for _, v := range l.GetListValue().Values {
						outputFormatsFields = c.traverseOutputFormatsFields(v.GetStructValue().GetFields()["properties"], fmt.Sprintf("%s%s.", prefix, key), outputFormatsFields)
					}
				}
				outputFormatsFields = c.traverseOutputFormatsFields(v.GetStructValue().GetFields()["properties"], fmt.Sprintf("%s%s.", prefix, key), outputFormatsFields)
			}

		}
	}

	return outputFormatsFields
}

// UsageHandlerCreator returns a function to initialize a UsageHandler. If the
// component doesn't have such function initialized, a no-op usage handler
// creator is returned.
func (c *Component) UsageHandlerCreator() UsageHandlerCreator {
	if c.NewUsageHandler == nil {
		return NewNoopUsageHandler
	}

	return c.NewUsageHandler
}

// Test is not implemented for the base component.
func (c *Component) Test(sysVars map[string]any, setup *structpb.Struct) error {
	return nil
}

// ReadFromGlobalConfig looks up a component credential field from a secret map
// that comes from the environment variable configuration.
//
// Config parameters are defined with snake_case, but the
// environment variable configuration loader replaces underscores by dots,
// so we can't use the parameter key directly.
// TODO using camelCase in configuration fields would fix this issue.
func ReadFromGlobalConfig(key string, secrets map[string]any) string {
	sanitized := strings.ReplaceAll(key, "-", "")
	if v, ok := secrets[sanitized].(string); ok {
		return v
	}

	return ""
}

// ComponentConfig is the config for the component.
type ComponentConfig struct {
	Task  string
	Input map[string]any
	Setup map[string]any
}
