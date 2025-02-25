// Code generated by MockGen. DO NOT EDIT.
// Source: directory.go

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	model "github.com/bsn-si/IPEHR-gateway/src/pkg/docs/model"
	processing "github.com/bsn-si/IPEHR-gateway/src/pkg/docs/service/processing"
	model0 "github.com/bsn-si/IPEHR-gateway/src/pkg/user/model"
	reflect "reflect"
	time "time"

	gomock "github.com/golang/mock/gomock"
	uuid "github.com/google/uuid"
)

// MockDirectoryService is a mock of DirectoryService interface.
type MockDirectoryService struct {
	ctrl     *gomock.Controller
	recorder *MockDirectoryServiceMockRecorder
}

// MockDirectoryServiceMockRecorder is the mock recorder for MockDirectoryService.
type MockDirectoryServiceMockRecorder struct {
	mock *MockDirectoryService
}

// NewMockDirectoryService creates a new mock instance.
func NewMockDirectoryService(ctrl *gomock.Controller) *MockDirectoryService {
	mock := &MockDirectoryService{ctrl: ctrl}
	mock.recorder = &MockDirectoryServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDirectoryService) EXPECT() *MockDirectoryServiceMockRecorder {
	return m.recorder
}

// Create mocks base method.
func (m *MockDirectoryService) Create(ctx context.Context, req processing.RequestInterface, systemID string, ehrUUID *uuid.UUID, user *model0.UserInfo, d *model.Directory) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Create", ctx, req, systemID, ehrUUID, user, d)
	ret0, _ := ret[0].(error)
	return ret0
}

// Create indicates an expected call of Create.
func (mr *MockDirectoryServiceMockRecorder) Create(ctx, req, systemID, ehrUUID, user, d interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Create", reflect.TypeOf((*MockDirectoryService)(nil).Create), ctx, req, systemID, ehrUUID, user, d)
}

// Delete mocks base method.
func (m *MockDirectoryService) Delete(ctx context.Context, req processing.RequestInterface, systemID string, ehrUUID *uuid.UUID, versionUID, userID string) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Delete", ctx, req, systemID, ehrUUID, versionUID, userID)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Delete indicates an expected call of Delete.
func (mr *MockDirectoryServiceMockRecorder) Delete(ctx, req, systemID, ehrUUID, versionUID, userID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Delete", reflect.TypeOf((*MockDirectoryService)(nil).Delete), ctx, req, systemID, ehrUUID, versionUID, userID)
}

// GetByID mocks base method.
func (m *MockDirectoryService) GetByID(ctx context.Context, userID, versionUID string) (*model.Directory, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetByID", ctx, userID, versionUID)
	ret0, _ := ret[0].(*model.Directory)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetByID indicates an expected call of GetByID.
func (mr *MockDirectoryServiceMockRecorder) GetByID(ctx, userID, versionUID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetByID", reflect.TypeOf((*MockDirectoryService)(nil).GetByID), ctx, userID, versionUID)
}

// GetByTime mocks base method.
func (m *MockDirectoryService) GetByTime(ctx context.Context, systemID string, ehrUUID *uuid.UUID, userID string, versionTime time.Time) (*model.Directory, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetByTime", ctx, systemID, ehrUUID, userID, versionTime)
	ret0, _ := ret[0].(*model.Directory)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetByTime indicates an expected call of GetByTime.
func (mr *MockDirectoryServiceMockRecorder) GetByTime(ctx, systemID, ehrUUID, userID, versionTime interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetByTime", reflect.TypeOf((*MockDirectoryService)(nil).GetByTime), ctx, systemID, ehrUUID, userID, versionTime)
}

// NewProcRequest mocks base method.
func (m *MockDirectoryService) NewProcRequest(reqID, userID, ehrUUID string, kind processing.RequestKind) (processing.RequestInterface, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewProcRequest", reqID, userID, ehrUUID, kind)
	ret0, _ := ret[0].(processing.RequestInterface)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewProcRequest indicates an expected call of NewProcRequest.
func (mr *MockDirectoryServiceMockRecorder) NewProcRequest(reqID, userID, ehrUUID, kind interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewProcRequest", reflect.TypeOf((*MockDirectoryService)(nil).NewProcRequest), reqID, userID, ehrUUID, kind)
}

// Update mocks base method.
func (m *MockDirectoryService) Update(ctx context.Context, req processing.RequestInterface, systemID string, ehrUUID *uuid.UUID, user *model0.UserInfo, d *model.Directory) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Update", ctx, req, systemID, ehrUUID, user, d)
	ret0, _ := ret[0].(error)
	return ret0
}

// Update indicates an expected call of Update.
func (mr *MockDirectoryServiceMockRecorder) Update(ctx, req, systemID, ehrUUID, user, d interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Update", reflect.TypeOf((*MockDirectoryService)(nil).Update), ctx, req, systemID, ehrUUID, user, d)
}
