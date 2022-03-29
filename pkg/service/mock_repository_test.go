// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/instill-ai/pipeline-backend/pkg/repository (interfaces: Repository)

// Package service_test is a generated GoMock package.
package service_test

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	datamodel "github.com/instill-ai/pipeline-backend/pkg/datamodel"
)

// MockRepository is a mock of Repository interface.
type MockRepository struct {
	ctrl     *gomock.Controller
	recorder *MockRepositoryMockRecorder
}

// MockRepositoryMockRecorder is the mock recorder for MockRepository.
type MockRepositoryMockRecorder struct {
	mock *MockRepository
}

// NewMockRepository creates a new mock instance.
func NewMockRepository(ctrl *gomock.Controller) *MockRepository {
	mock := &MockRepository{ctrl: ctrl}
	mock.recorder = &MockRepositoryMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockRepository) EXPECT() *MockRepositoryMockRecorder {
	return m.recorder
}

// CreatePipeline mocks base method.
func (m *MockRepository) CreatePipeline(arg0 datamodel.Pipeline) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreatePipeline", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// CreatePipeline indicates an expected call of CreatePipeline.
func (mr *MockRepositoryMockRecorder) CreatePipeline(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreatePipeline", reflect.TypeOf((*MockRepository)(nil).CreatePipeline), arg0)
}

// DeletePipeline mocks base method.
func (m *MockRepository) DeletePipeline(arg0, arg1 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeletePipeline", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeletePipeline indicates an expected call of DeletePipeline.
func (mr *MockRepositoryMockRecorder) DeletePipeline(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeletePipeline", reflect.TypeOf((*MockRepository)(nil).DeletePipeline), arg0, arg1)
}

// GetPipelineByName mocks base method.
func (m *MockRepository) GetPipelineByName(arg0, arg1 string) (datamodel.Pipeline, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPipelineByName", arg0, arg1)
	ret0, _ := ret[0].(datamodel.Pipeline)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetPipelineByName indicates an expected call of GetPipelineByName.
func (mr *MockRepositoryMockRecorder) GetPipelineByName(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPipelineByName", reflect.TypeOf((*MockRepository)(nil).GetPipelineByName), arg0, arg1)
}

// ListPipelines mocks base method.
func (m *MockRepository) ListPipelines(arg0 datamodel.ListPipelineQuery) ([]datamodel.Pipeline, uint64, uint64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListPipelines", arg0)
	ret0, _ := ret[0].([]datamodel.Pipeline)
	ret1, _ := ret[1].(uint64)
	ret2, _ := ret[2].(uint64)
	ret3, _ := ret[3].(error)
	return ret0, ret1, ret2, ret3
}

// ListPipelines indicates an expected call of ListPipelines.
func (mr *MockRepositoryMockRecorder) ListPipelines(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListPipelines", reflect.TypeOf((*MockRepository)(nil).ListPipelines), arg0)
}

// UpdatePipeline mocks base method.
func (m *MockRepository) UpdatePipeline(arg0 datamodel.Pipeline) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdatePipeline", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdatePipeline indicates an expected call of UpdatePipeline.
func (mr *MockRepositoryMockRecorder) UpdatePipeline(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdatePipeline", reflect.TypeOf((*MockRepository)(nil).UpdatePipeline), arg0)
}