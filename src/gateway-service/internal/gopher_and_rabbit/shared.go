package gopher_and_rabbit

type Configuration struct {
	AMQPConnectionURL string
}

type AddTask struct {
	Addr      string
	Username  string
	TicketUID string
}

var Config = Configuration{
	AMQPConnectionURL: "amqp://guest:guest@rabbitmq:5672/",
}
