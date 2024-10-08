// Code generated by MockGen. DO NOT EDIT.
// Source: /home/aawilson/go/src/ais-k8s/operator/pkg/services/client_manager.go

// Package mock_services is a generated GoMock package.
package mock_services

import (
	context "context"
	reflect "reflect"

	v1beta1 "github.com/ais-operator/api/v1beta1"
	services "github.com/ais-operator/pkg/services"
	gomock "github.com/golang/mock/gomock"
)

// MockAISClientManagerInterface is a mock of AISClientManagerInterface interface.
type MockAISClientManagerInterface struct {
	ctrl     *gomock.Controller
	recorder *MockAISClientManagerInterfaceMockRecorder
}

// MockAISClientManagerInterfaceMockRecorder is the mock recorder for MockAISClientManagerInterface.
type MockAISClientManagerInterfaceMockRecorder struct {
	mock *MockAISClientManagerInterface
}

// NewMockAISClientManagerInterface creates a new mock instance.
func NewMockAISClientManagerInterface(ctrl *gomock.Controller) *MockAISClientManagerInterface {
	mock := &MockAISClientManagerInterface{ctrl: ctrl}
	mock.recorder = &MockAISClientManagerInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockAISClientManagerInterface) EXPECT() *MockAISClientManagerInterfaceMockRecorder {
	return m.recorder
}

// GetClient mocks base method.
func (m *MockAISClientManagerInterface) GetClient(ctx context.Context, ais *v1beta1.AIStore) (services.AIStoreClientInterface, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetClient", ctx, ais)
	ret0, _ := ret[0].(services.AIStoreClientInterface)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetClient indicates an expected call of GetClient.
func (mr *MockAISClientManagerInterfaceMockRecorder) GetClient(ctx, ais interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetClient", reflect.TypeOf((*MockAISClientManagerInterface)(nil).GetClient), ctx, ais)
}

// GetPrimaryClient mocks base method.
func (m *MockAISClientManagerInterface) GetPrimaryClient(ctx context.Context, ais *v1beta1.AIStore) (services.AIStoreClientInterface, bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPrimaryClient", ctx, ais)
	ret0, _ := ret[0].(services.AIStoreClientInterface)
	ret1, _ := ret[1].(bool)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetPrimaryClient indicates an expected call of GetPrimaryClient.
func (mr *MockAISClientManagerInterfaceMockRecorder) GetPrimaryClient(ctx, ais interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPrimaryClient", reflect.TypeOf((*MockAISClientManagerInterface)(nil).GetPrimaryClient), ctx, ais)
}
