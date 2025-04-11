package rabbitmqclient

import (
	"encoding/json"
	"net/http"

	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	"github.com/pkg/errors"
)

type RabbitMQClient interface {
	// Permissions
	UpdatePermissionsIn(vhost, username string, permissions rabbithole.Permissions) (res *http.Response, err error)
	GetPermissionsIn(vhost, username string) (rec rabbithole.PermissionInfo, err error)
	ClearPermissionsIn(vhost, username string) (res *http.Response, err error)
	// Exchange
	GetExchange(vhost, exchange string) (rec *rabbithole.DetailedExchangeInfo, err error)
	DeclareExchange(vhost, exchange string, info rabbithole.ExchangeSettings) (res *http.Response, err error)
	DeleteExchange(vhost, exchange string) (res *http.Response, err error)
	// Vhost
	GetVhost(vhostName string) (rec *rabbithole.VhostInfo, err error)
	PutVhost(vhostName string, settings rabbithole.VhostSettings) (res *http.Response, err error)
	DeleteVhost(vhostName string) (res *http.Response, err error)
	// User
	GetUser(username string) (rec *rabbithole.UserInfo, err error)
	PutUser(username string, info rabbithole.UserSettings) (res *http.Response, err error)
	PutUserWithoutPassword(username string, info rabbithole.UserSettings) (res *http.Response, err error)
	DeleteUser(username string) (res *http.Response, err error)
	// Binding
	ListQueueBindingsBetween(vhost, exchange string, queue string) (rec []rabbithole.BindingInfo, err error)
	ListExchangeBindingsBetween(vhost, source string, destination string) (rec []rabbithole.BindingInfo, err error)
	DeclareBinding(vhost string, info rabbithole.BindingInfo) (res *http.Response, err error)
	DeleteBinding(vhost string, info rabbithole.BindingInfo) (res *http.Response, err error)
	ListBindingsIn(vhost string) (rec []rabbithole.BindingInfo, err error)
	// Queue
	GetQueue(vhost, queue string) (rec *rabbithole.DetailedQueueInfo, err error)
	DeclareQueue(vhost, queue string, info rabbithole.QueueSettings) (res *http.Response, err error)
	DeleteQueue(vhost, queue string, opts ...rabbithole.QueueDeleteOptions) (res *http.Response, err error)
}

type RabbitMqService struct {
	Rmqc RabbitMQClient
}

type RabbitMqCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Endpoint string `json:"endpoint"`
}

func NewClient(creds []byte) (*RabbitMqService, error) {
	var config = new(RabbitMqCredentials)
	if err := json.Unmarshal(creds, &config); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal credentials")
	}
	// fmt.Printf("RabbitMq address: %s\n", config.Endpoint)
	c, err := rabbithole.NewClient(config.Endpoint, config.Username, config.Password)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create RabbitMQ client")
	}
	return &RabbitMqService{
		Rmqc: c,
	}, err
}

func IsNotFoundError(err error) bool {
	var errResp rabbithole.ErrorResponse
	return errors.As(err, &errResp) && errResp.StatusCode == http.StatusNotFound
}
