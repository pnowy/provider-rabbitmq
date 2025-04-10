package fake

import (
	"github.com/michaelklishin/rabbit-hole/v3"
	"net/http"
)

type MockClient struct {
	MockUpdatePermissionsIn         func(vhost, username string, permissions rabbithole.Permissions) (res *http.Response, err error)
	MockGetPermissionsIn            func(vhost, username string) (rec rabbithole.PermissionInfo, err error)
	MockClearPermissionsIn          func(vhost, username string) (res *http.Response, err error)
	MockGetExchange                 func(vhost, exchange string) (res *http.Response, err error)
	MockDeclareExchange             func(vhost, exchange string, info rabbithole.ExchangeSettings) (res *http.Response, err error)
	MockDeleteExchange              func(vhost, exchange string) (res *http.Response, err error)
	MockListQueueBindingsBetween    func(vhost, exchange string, queue string) (rec []rabbithole.BindingInfo, err error)
	MockListExchangeBindingsBetween func(vhost, source string, destination string) (rec []rabbithole.BindingInfo, err error)
	MockDeclareBinding              func(vhost string, info rabbithole.BindingInfo) (res *http.Response, err error)
	MockDeleteBinding               func(vhost string, info rabbithole.BindingInfo) (res *http.Response, err error)
	MockListBindingsIn              func(vhost string) (rec []rabbithole.BindingInfo, err error)
	MockGetUser                     func(username string) (rec *rabbithole.UserInfo, err error)
	MockPutUser                     func(username string, info rabbithole.UserSettings) (res *http.Response, err error)
	MockPutUserWithoutPassword      func(username string, info rabbithole.UserSettings) (res *http.Response, err error)
	MockDeleteUser                  func(username string) (res *http.Response, err error)
	MockGetVhost                    func(vhostname string) (rec *rabbithole.VhostInfo, err error)
	MockPutHost                     func(vhostname string, settings rabbithole.VhostSettings) (res *http.Response, err error)
	MockDeleteVhost                 func(vhost string) (res *http.Response, err error)
}

// Permissions

func (m MockClient) UpdatePermissionsIn(vhost, username string, permissions rabbithole.Permissions) (res *http.Response, err error) {
	return m.MockUpdatePermissionsIn(vhost, username, permissions)
}

func (m MockClient) GetPermissionsIn(vhost, username string) (rec rabbithole.PermissionInfo, err error) {
	rec = rabbithole.PermissionInfo{
		User:  username,
		Vhost: vhost,
	}
	return rec, err
}

func (m MockClient) ClearPermissionsIn(vhost, username string) (res *http.Response, err error) {
	return m.MockClearPermissionsIn(vhost, username)
}

// Exchange

func (m MockClient) GetExchange(vhost, exchange string) (rec *rabbithole.DetailedExchangeInfo, err error) {
	return m.GetExchange(vhost, exchange)
}

func (m MockClient) DeclareExchange(vhost, exchange string, info rabbithole.ExchangeSettings) (res *http.Response, err error) {
	return m.MockDeclareExchange(vhost, exchange, info)
}
func (m MockClient) DeleteExchange(vhost, exchange string) (res *http.Response, err error) {
	return m.MockDeleteExchange(vhost, exchange)
}

// Binding

func (m MockClient) ListQueueBindingsBetween(vhost, exchange string, queue string) (rec []rabbithole.BindingInfo, err error) {
	return m.MockListQueueBindingsBetween(vhost, exchange, queue)
}

func (m MockClient) DeclareBinding(vhost string, info rabbithole.BindingInfo) (res *http.Response, err error) {
	return m.MockDeclareBinding(vhost, info)
}

func (m MockClient) DeleteBinding(vhost string, info rabbithole.BindingInfo) (res *http.Response, err error) {
	return m.MockDeleteBinding(vhost, info)
}

func (m MockClient) ListBindingsIn(vhost string) (rec []rabbithole.BindingInfo, err error) {
	return m.MockListBindingsIn(vhost)
}

func (m MockClient) ListExchangeBindingsBetween(vhost, source string, destination string) (rec []rabbithole.BindingInfo, err error) {
	return m.MockListExchangeBindingsBetween(vhost, source, destination)
}

// User

func (m MockClient) GetUser(username string) (rec *rabbithole.UserInfo, err error) {
	return m.MockGetUser(username)
}

func (m MockClient) PutUser(username string, info rabbithole.UserSettings) (res *http.Response, err error) {
	return m.MockPutUser(username, info)
}

func (m MockClient) PutUserWithoutPassword(username string, info rabbithole.UserSettings) (res *http.Response, err error) {
	return m.MockPutUserWithoutPassword(username, info)
}

func (m MockClient) DeleteUser(username string) (res *http.Response, err error) {
	return m.MockDeleteUser(username)
}

// Vhost

func (m MockClient) GetVhost(vhostname string) (rec *rabbithole.VhostInfo, err error) {
	return m.MockGetVhost(vhostname)
}

func (m MockClient) PutVhost(vhostname string, settings rabbithole.VhostSettings) (res *http.Response, err error) {
	return m.MockPutHost(vhostname, settings)
}

func (m MockClient) DeleteVhost(vhostname string) (res *http.Response, err error) {
	return m.MockDeleteVhost(vhostname)
}
