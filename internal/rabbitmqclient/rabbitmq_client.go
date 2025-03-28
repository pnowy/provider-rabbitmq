package rabbitmqclient

import (
	"encoding/json"
	"fmt"

	rabbithole "github.com/michaelklishin/rabbit-hole/v3"
	"github.com/pkg/errors"
)

type RabbitMqService struct {
	Rmqc *rabbithole.Client
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
	fmt.Printf("RabbitMq address: %s\n", config.Endpoint)
	c, err := rabbithole.NewClient(config.Endpoint, config.Username, config.Password)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create RabbitMQ client")
	}
	return &RabbitMqService{
		Rmqc: c,
	}, err
}
