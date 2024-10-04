// Code generated by MockGen. DO NOT EDIT.
// Source: /home/aawilson/go/src/ais-k8s/operator/pkg/services/aisapi.go

// Package mock_services is a generated GoMock package.
package mock_services

import (
	reflect "reflect"

	apc "github.com/NVIDIA/aistore/api/apc"
	cmn "github.com/NVIDIA/aistore/cmn"
	meta "github.com/NVIDIA/aistore/core/meta"
	v1beta1 "github.com/ais-operator/api/v1beta1"
	gomock "github.com/golang/mock/gomock"
)

// MockAIStoreClientInterface is a mock of AIStoreClientInterface interface.
type MockAIStoreClientInterface struct {
	ctrl     *gomock.Controller
	recorder *MockAIStoreClientInterfaceMockRecorder
}

// MockAIStoreClientInterfaceMockRecorder is the mock recorder for MockAIStoreClientInterface.
type MockAIStoreClientInterfaceMockRecorder struct {
	mock *MockAIStoreClientInterface
}

// NewMockAIStoreClientInterface creates a new mock instance.
func NewMockAIStoreClientInterface(ctrl *gomock.Controller) *MockAIStoreClientInterface {
	mock := &MockAIStoreClientInterface{ctrl: ctrl}
	mock.recorder = &MockAIStoreClientInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockAIStoreClientInterface) EXPECT() *MockAIStoreClientInterfaceMockRecorder {
	return m.recorder
}

// DecommissionCluster mocks base method.
func (m *MockAIStoreClientInterface) DecommissionCluster(rmUserData bool) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DecommissionCluster", rmUserData)
	ret0, _ := ret[0].(error)
	return ret0
}

// DecommissionCluster indicates an expected call of DecommissionCluster.
func (mr *MockAIStoreClientInterfaceMockRecorder) DecommissionCluster(rmUserData interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DecommissionCluster", reflect.TypeOf((*MockAIStoreClientInterface)(nil).DecommissionCluster), rmUserData)
}

// DecommissionNode mocks base method.
func (m *MockAIStoreClientInterface) DecommissionNode(actValue *apc.ActValRmNode) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DecommissionNode", actValue)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DecommissionNode indicates an expected call of DecommissionNode.
func (mr *MockAIStoreClientInterfaceMockRecorder) DecommissionNode(actValue interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DecommissionNode", reflect.TypeOf((*MockAIStoreClientInterface)(nil).DecommissionNode), actValue)
}

// GetAuthToken mocks base method.
func (m *MockAIStoreClientInterface) GetAuthToken() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetAuthToken")
	ret0, _ := ret[0].(string)
	return ret0
}

// GetAuthToken indicates an expected call of GetAuthToken.
func (mr *MockAIStoreClientInterfaceMockRecorder) GetAuthToken() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetAuthToken", reflect.TypeOf((*MockAIStoreClientInterface)(nil).GetAuthToken))
}

// GetClusterMap mocks base method.
func (m *MockAIStoreClientInterface) GetClusterMap() (*meta.Smap, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetClusterMap")
	ret0, _ := ret[0].(*meta.Smap)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetClusterMap indicates an expected call of GetClusterMap.
func (mr *MockAIStoreClientInterfaceMockRecorder) GetClusterMap() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetClusterMap", reflect.TypeOf((*MockAIStoreClientInterface)(nil).GetClusterMap))
}

// HasValidBaseParams mocks base method.
func (m *MockAIStoreClientInterface) HasValidBaseParams(ais *v1beta1.AIStore) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HasValidBaseParams", ais)
	ret0, _ := ret[0].(bool)
	return ret0
}

// HasValidBaseParams indicates an expected call of HasValidBaseParams.
func (mr *MockAIStoreClientInterfaceMockRecorder) HasValidBaseParams(ais interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HasValidBaseParams", reflect.TypeOf((*MockAIStoreClientInterface)(nil).HasValidBaseParams), ais)
}

// Health mocks base method.
func (m *MockAIStoreClientInterface) Health(isPrimary bool) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Health", isPrimary)
	ret0, _ := ret[0].(error)
	return ret0
}

// Health indicates an expected call of Health.
func (mr *MockAIStoreClientInterfaceMockRecorder) Health(isPrimary interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Health", reflect.TypeOf((*MockAIStoreClientInterface)(nil).Health), isPrimary)
}

// SetClusterConfigUsingMsg mocks base method.
func (m *MockAIStoreClientInterface) SetClusterConfigUsingMsg(configToUpdate *cmn.ConfigToSet, transient bool) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetClusterConfigUsingMsg", configToUpdate, transient)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetClusterConfigUsingMsg indicates an expected call of SetClusterConfigUsingMsg.
func (mr *MockAIStoreClientInterfaceMockRecorder) SetClusterConfigUsingMsg(configToUpdate, transient interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetClusterConfigUsingMsg", reflect.TypeOf((*MockAIStoreClientInterface)(nil).SetClusterConfigUsingMsg), configToUpdate, transient)
}

// SetPrimaryProxy mocks base method.
func (m *MockAIStoreClientInterface) SetPrimaryProxy(newPrimaryID string, force bool) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetPrimaryProxy", newPrimaryID, force)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetPrimaryProxy indicates an expected call of SetPrimaryProxy.
func (mr *MockAIStoreClientInterfaceMockRecorder) SetPrimaryProxy(newPrimaryID, force interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetPrimaryProxy", reflect.TypeOf((*MockAIStoreClientInterface)(nil).SetPrimaryProxy), newPrimaryID, force)
}

// ShutdownCluster mocks base method.
func (m *MockAIStoreClientInterface) ShutdownCluster() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ShutdownCluster")
	ret0, _ := ret[0].(error)
	return ret0
}

// ShutdownCluster indicates an expected call of ShutdownCluster.
func (mr *MockAIStoreClientInterfaceMockRecorder) ShutdownCluster() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ShutdownCluster", reflect.TypeOf((*MockAIStoreClientInterface)(nil).ShutdownCluster))
}