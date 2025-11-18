package grpcapi

import (
	"context"
	"encoding/json"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"

	"github.com/chaosblade-io/chaosblade/pkg/service/experiment"
)

// ExperimentService exposes experiment operations over gRPC using a JSON codec to avoid protobuf generation.
type ExperimentService struct {
	svc *experiment.Service
}

// NewServer initializes a gRPC server and registers experiment handlers.
func NewServer(svc *experiment.Service, opts ...grpc.ServerOption) *grpc.Server {
	encoding.RegisterCodec(jsonCodec{})
	server := grpc.NewServer(opts...)
	RegisterExperimentService(server, &ExperimentService{svc: svc})
	return server
}

// ListenAndServe starts the gRPC server on the provided address.
func ListenAndServe(addr string, svc *experiment.Service, opts ...grpc.ServerOption) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := NewServer(svc, opts...)
	return server.Serve(lis)
}

// Create triggers a new experiment.
func (e *ExperimentService) Create(ctx context.Context, request *experiment.CreateExperimentRequest) (*ExperimentResponse, error) {
	resp, record, err := e.svc.Create(ctx, *request)
	if err != nil {
		return nil, err
	}
	return &ExperimentResponse{Uid: record.Uid, Success: resp.Success, Code: int32(resp.Code), Error: resp.Err}, nil
}

// Destroy removes an experiment.
func (e *ExperimentService) Destroy(ctx context.Context, request *experiment.DestroyExperimentRequest) (*ExperimentResponse, error) {
	resp, err := e.svc.Destroy(ctx, *request)
	if err != nil {
		return nil, err
	}
	return &ExperimentResponse{Uid: request.UID, Success: resp.Success, Code: int32(resp.Code), Error: resp.Err}, nil
}

// Query returns the stored experiment record.
func (e *ExperimentService) Query(ctx context.Context, request *ExperimentQuery) (*ExperimentRecord, error) {
	record, err := e.svc.Query(request.Uid)
	if err != nil {
		return nil, err
	}
	return &ExperimentRecord{Uid: record.Uid, Command: record.Command, SubCommand: record.SubCommand, Flag: record.Flag, Status: record.Status, Error: record.Error, CreateTime: record.CreateTime, UpdateTime: record.UpdateTime}, nil
}

// ExperimentResponse is a lightweight response wrapper for gRPC clients.
type ExperimentResponse struct {
	Uid     string `json:"uid"`
	Success bool   `json:"success"`
	Code    int32  `json:"code"`
	Error   string `json:"error"`
}

// ExperimentQuery describes a query request.
type ExperimentQuery struct {
	Uid string `json:"uid"`
}

// ExperimentRecord mirrors the datastore structure for gRPC responses.
type ExperimentRecord struct {
	Uid        string `json:"uid"`
	Command    string `json:"command"`
	SubCommand string `json:"subCommand"`
	Flag       string `json:"flag"`
	Status     string `json:"status"`
	Error      string `json:"error"`
	CreateTime string `json:"createTime"`
	UpdateTime string `json:"updateTime"`
}

// jsonCodec allows using JSON as the gRPC payload format.
type jsonCodec struct{}

func (jsonCodec) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (jsonCodec) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func (jsonCodec) Name() string { return "json" }

// RegisterExperimentService wires handlers into the gRPC server using a manual ServiceDesc to avoid generated code.
func RegisterExperimentService(server *grpc.Server, service *ExperimentService) {
	server.RegisterService(&_ExperimentService_serviceDesc, service)
}

var _ExperimentService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "chaosblade.v1.ExperimentService",
	HandlerType: (*ExperimentService)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Create",
			Handler:    _ExperimentService_Create_Handler,
		},
		{
			MethodName: "Destroy",
			Handler:    _ExperimentService_Destroy_Handler,
		},
		{
			MethodName: "Query",
			Handler:    _ExperimentService_Query_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "chaosblade/experiments",
}

func _ExperimentService_Create_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(experiment.CreateExperimentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*ExperimentService).Create(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/chaosblade.v1.ExperimentService/Create",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*ExperimentService).Create(ctx, req.(*experiment.CreateExperimentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ExperimentService_Destroy_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(experiment.DestroyExperimentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*ExperimentService).Destroy(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/chaosblade.v1.ExperimentService/Destroy",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*ExperimentService).Destroy(ctx, req.(*experiment.DestroyExperimentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ExperimentService_Query_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ExperimentQuery)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*ExperimentService).Query(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/chaosblade.v1.ExperimentService/Query",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*ExperimentService).Query(ctx, req.(*ExperimentQuery))
	}
	return interceptor(ctx, in, info, handler)
}
