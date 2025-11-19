package preparation

import (
	"context"
	"strings"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade/data"
	"github.com/chaosblade-io/chaosblade/exec/jvm"
	"github.com/chaosblade-io/chaosblade/pkg/service/status"
)

// PrepareRequest describes a preparation payload for supported program types.
type PrepareRequest struct {
	Type        string `json:"type"`
	Process     string `json:"process"`
	PID         string `json:"pid"`
	JavaHome    string `json:"javaHome"`
	Async       bool   `json:"async"`
	Endpoint    string `json:"endpoint"`
	UID         string `json:"uid"`
	Refresh     bool   `json:"refresh"`
	SandboxPort string `json:"port"`
}

// RevokeRequest describes a detach payload.
type RevokeRequest struct {
	UID     string `json:"uid"`
	Type    string `json:"type"`
	Process string `json:"process"`
	PID     string `json:"pid"`
}

// Service exposes preparation lifecycle operations.
type Service struct {
	ds data.SourceI
}

// New creates a preparation service backed by the shared datasource.
func New(ds data.SourceI) *Service {
	return &Service{ds: ds}
}

// Prepare installs the runtime attachment for supported program types.
func (s *Service) Prepare(ctx context.Context, req PrepareRequest) (*spec.Response, *data.PreparationRecord, error) {
	switch strings.ToLower(req.Type) {
	case "jvm", "java":
		resp, _ := jvm.Prepare(ctx, req.Process, req.PID, req.JavaHome)
		if !resp.Success {
			return resp, nil, nil
		}
		uid, _ := resp.Result.(string)
		record, err := s.ds.QueryPreparationByUid(uid)
		if err != nil {
			return nil, nil, spec.ResponseFailWithFlags(spec.DatabaseError, "query", err)
		}
		return resp, record, nil
	default:
		return nil, nil, spec.ResponseFailWithFlags(spec.ParameterIllegal, "type", req.Type, "not support the type")
	}
}

// Revoke detaches the preparation artifacts based on uid.
func (s *Service) Revoke(ctx context.Context, req RevokeRequest) (*spec.Response, error) {
	if req.UID == "" {
		return nil, spec.ResponseFailWithFlags(spec.ParameterLess, "uid")
	}
	record, err := s.ds.QueryPreparationByUid(req.UID)
	if err != nil {
		return nil, spec.ResponseFailWithFlags(spec.DatabaseError, "query", err)
	}
	if record == nil {
		return nil, spec.ResponseFailWithFlags(spec.DataNotFound, req.UID)
	}

	var resp *spec.Response
	switch strings.ToLower(record.ProgramType) {
	case "jvm", "java":
		resp = jvm.Revoke(ctx, record, record.Process, record.Pid)
	default:
		return nil, spec.ResponseFailWithFlags(spec.ParameterIllegal, "type", record.ProgramType, "not support the type")
	}

	if resp.Success || strings.Contains(resp.Err, "connection refused") {
		_ = s.ds.UpdatePreparationRecordByUid(record.Uid, status.Revoked, "")
		if !resp.Success {
			// reset to success when sandbox already detached
			resp = spec.ReturnSuccess("success")
		}
		return resp, nil
	}

	_ = s.ds.UpdatePreparationRecordByUid(record.Uid, record.Status, resp.Err)
	return resp, nil
}
