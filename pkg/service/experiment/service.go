package experiment

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-spec-go/util"

	"github.com/chaosblade-io/chaosblade/data"
	"github.com/chaosblade-io/chaosblade/pkg/service/dispatcher"
	"github.com/chaosblade-io/chaosblade/pkg/service/status"
)

// CreateExperimentRequest describes a REST/gRPC payload for creating an experiment.
type CreateExperimentRequest struct {
	UID         string            `json:"uid"`
	Scope       string            `json:"scope"`
	Target      string            `json:"target"` // target executor name
	Action      string            `json:"action"` // experiment action
	Flags       map[string]string `json:"flags"`
	Description string            `json:"description"`
}

// DestroyExperimentRequest describes a destroy payload.
type DestroyExperimentRequest struct {
	UID    string            `json:"uid"`
	Scope  string            `json:"scope"`
	Target string            `json:"target"`
	Action string            `json:"action"`
	Flags  map[string]string `json:"flags"`
}

// StatusQuery encapsulates status request parameters for preparations and experiments.
type StatusQuery struct {
	Type   string `json:"type"`
	Target string `json:"target"`
	Action string `json:"action"`
	Flag   string `json:"flag"`
	Limit  string `json:"limit"`
	Status string `json:"status"`
	UID    string `json:"uid"`
	Asc    bool   `json:"asc"`
}

// Service wraps the dispatcher and data source.
type Service struct {
	dispatcher *dispatcher.Dispatcher
	ds         data.SourceI
}

// New creates an experiment service with the given dispatcher and datasource.
func New(d *dispatcher.Dispatcher, ds data.SourceI) *Service {
	return &Service{dispatcher: d, ds: ds}
}

// Create invokes the dispatcher to start an experiment and records it in the datastore.
func (s *Service) Create(ctx context.Context, request CreateExperimentRequest) (*spec.Response, *data.ExperimentModel, error) {
	if request.Target == "" || request.Action == "" {
		return nil, nil, spec.ResponseFailWithFlags(spec.ParameterLess, "target|action")
	}
	uid := request.UID
	if uid == "" {
		var err error
		uid, err = util.GenerateUid()
		if err != nil {
			return nil, nil, spec.ResponseFailWithFlags(spec.GenerateUidFailed, err)
		}
	}

	flags := request.Flags
	if flags == nil {
		flags = make(map[string]string)
	}

	expModel := &spec.ExpModel{
		Target:      request.Target,
		Scope:       request.Scope,
		ActionName:  request.Action,
		ActionFlags: flags,
	}

	commandModel, err := s.persistExperiment(uid, request.Scope, request.Target, request.Action, expModel)
	if err != nil {
		return nil, nil, err
	}

	ctx = context.WithValue(ctx, spec.Uid, uid)
	response, err := s.dispatcher.Dispatch(ctx, dispatcher.ExecutionRequest{
		Scope:   request.Scope,
		Target:  request.Target,
		Action:  request.Action,
		UID:     uid,
		Model:   expModel,
		Destroy: false,
	})
	if err != nil {
		return nil, nil, err
	}

	if !response.Success {
		_ = s.ds.UpdateExperimentModelByUid(uid, status.Error, response.Err)
	} else {
		_ = s.ds.UpdateExperimentModelByUid(uid, status.Success, "")
		response.Result = uid
	}
	return response, commandModel, nil
}

// Destroy removes an experiment by uid. If no record is found and scope/target/action are provided, the dispatcher will still be invoked.
func (s *Service) Destroy(ctx context.Context, request DestroyExperimentRequest) (*spec.Response, error) {
	if request.UID == "" {
		return nil, spec.ResponseFailWithFlags(spec.ParameterLess, "uid")
	}
	record, err := s.ds.QueryExperimentModelByUid(request.UID)
	if err != nil {
		return nil, spec.ResponseFailWithFlags(spec.DatabaseError, "query", err)
	}

	var expModel *spec.ExpModel
	var scope, target, action string
	switch {
	case record != nil:
		scope, target, action, expModel, err = convertRecordToModel(record)
		if err != nil {
			return nil, spec.ResponseFailWithFlags(spec.HandlerExecNotFound, err.Error())
		}
	case request.Target != "" && request.Action != "":
		expModel = &spec.ExpModel{
			Target:      request.Target,
			Scope:       request.Scope,
			ActionName:  request.Action,
			ActionFlags: request.Flags,
		}
		scope, target, action = request.Scope, request.Target, request.Action
	default:
		return nil, spec.ResponseFailWithFlags(spec.DataNotFound, request.UID)
	}

	ctx = spec.SetDestroyFlag(ctx, request.UID)
	response, err := s.dispatcher.Dispatch(ctx, dispatcher.ExecutionRequest{
		Scope:   scope,
		Target:  target,
		Action:  action,
		UID:     request.UID,
		Model:   expModel,
		Destroy: true,
	})
	if err != nil {
		return nil, err
	}
	if response.Success {
		_ = s.ds.UpdateExperimentModelByUid(request.UID, status.Destroyed, "")
	}
	return response, nil
}

// Status queries experiment or preparation state, mirroring the CLI semantics.
func (s *Service) Status(ctx context.Context, request StatusQuery) (*spec.Response, error) {
	uid := request.UID
	switch strings.ToLower(request.Type) {
	case "create", "destroy", "c", "d":
		if uid != "" {
			record, err := s.ds.QueryExperimentModelByUid(uid)
			if err != nil {
				return nil, spec.ResponseFailWithFlags(spec.DatabaseError, "query", err)
			}
			if record == nil {
				return nil, spec.ResponseFailWithFlags(spec.DataNotFound, uid)
			}
			return spec.ReturnSuccess(record), nil
		}
		models, err := s.ds.QueryExperimentModels(request.Target, request.Action, request.Flag, request.Status, request.Limit, request.Asc)
		if err != nil {
			return nil, spec.ResponseFailWithFlags(spec.DatabaseError, "query", err)
		}
		return spec.ReturnSuccess(models), nil
	case "prepare", "revoke", "p", "r":
		if uid != "" {
			record, err := s.ds.QueryPreparationByUid(uid)
			if err != nil {
				return nil, spec.ResponseFailWithFlags(spec.DatabaseError, "query", err)
			}
			if record == nil {
				return nil, spec.ResponseFailWithFlags(spec.DataNotFound, uid)
			}
			return spec.ReturnSuccess(record), nil
		}
		records, err := s.ds.QueryPreparationRecords(request.Target, request.Status, request.Action, request.Flag, request.Limit, request.Asc)
		if err != nil {
			return nil, spec.ResponseFailWithFlags(spec.DatabaseError, "query", err)
		}
		return spec.ReturnSuccess(records), nil
	default:
		if uid == "" {
			return nil, spec.ResponseFailWithFlags(spec.ParameterLess, "type|uid, must specify the right type or uid")
		}
		record, err := s.ds.QueryExperimentModelByUid(uid)
		if err != nil {
			return nil, spec.ResponseFailWithFlags(spec.DatabaseError, "query", err)
		}
		if !util.IsNil(record) {
			return spec.ReturnSuccess(record), nil
		}
		preparation, err := s.ds.QueryPreparationByUid(uid)
		if err != nil {
			return nil, spec.ResponseFailWithFlags(spec.DatabaseError, "query", err)
		}
		if util.IsNil(preparation) {
			return nil, spec.ResponseFailWithFlags(spec.DataNotFound, uid)
		}
		return spec.ReturnSuccess(preparation), nil
	}
}

// Query returns the stored experiment information.
func (s *Service) Query(uid string) (*data.ExperimentModel, error) {
	if uid == "" {
		return nil, spec.ResponseFailWithFlags(spec.ParameterLess, "uid")
	}
	model, err := s.ds.QueryExperimentModelByUid(uid)
	if err != nil {
		return nil, spec.ResponseFailWithFlags(spec.DatabaseError, "query", err)
	}
	if model == nil {
		return nil, spec.ResponseFailWithFlags(spec.DataNotFound, uid)
	}
	return model, nil
}

func (s *Service) persistExperiment(uid, scope, target, action string, expModel *spec.ExpModel) (*data.ExperimentModel, error) {
	flagsInline := spec.ConvertExpMatchersToString(expModel, func() map[string]spec.Empty {
		return make(map[string]spec.Empty)
	})
	commandPath := buildCommandPath(scope, target, action)
	createTime := time.Now().Format(time.RFC3339Nano)
	model := &data.ExperimentModel{
		Uid:        uid,
		Command:    targetForRecord(scope, target),
		SubCommand: subCommandForRecord(scope, target, action),
		Flag:       flagsInline,
		Status:     status.Created,
		Error:      "",
		CreateTime: createTime,
		UpdateTime: createTime,
	}
	if err := s.ds.InsertExperimentModel(model); err != nil {
		return nil, spec.ResponseFailWithFlags(spec.DatabaseError, "insert", err)
	}
	_ = commandPath // placeholder for compatibility, retained for auditing needs
	return model, nil
}

func buildCommandPath(scope, target, action string) string {
	parts := []string{"blade", "create"}
	if scope != "" {
		parts = append(parts, scope)
	}
	parts = append(parts, target, action)
	return strings.Join(parts, " ")
}

func targetForRecord(scope, target string) string {
	if scope != "" {
		return scope
	}
	return target
}

func subCommandForRecord(scope, target, action string) string {
	if scope != "" {
		return fmt.Sprintf("%s %s", target, action)
	}
	return action
}

func convertRecordToModel(record *data.ExperimentModel) (string, string, string, *spec.ExpModel, error) {
	subCommands := strings.Split(record.SubCommand, " ")
	if len(subCommands) == 0 {
		return "", "", "", nil, fmt.Errorf("invalid sub command for uid %s", record.Uid)
	}
	action := subCommands[len(subCommands)-1]
	target := record.Command
	scope := ""
	if len(subCommands) > 1 {
		target = subCommands[len(subCommands)-2]
		scope = record.Command
	}
	expModel := spec.ConvertCommandsToExpModel(action, target, record.Flag)
	expModel.Scope = scope
	return scope, target, action, expModel, nil
}
