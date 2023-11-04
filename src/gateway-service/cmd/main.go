package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"lab2/src/gateway-service/internal/gopher_and_rabbit"
	"lab2/src/gateway-service/internal/handlers"
	"lab2/src/gateway-service/internal/service"

	"github.com/streadway/amqp"
)

func handleError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}

}

func main() {

	conn, err := amqp.Dial(gopher_and_rabbit.Config.AMQPConnectionURL)
	handleError(err, "Can't connect to AMQP")
	defer conn.Close()

	amqpChannel, err := conn.Channel()
	handleError(err, "Can't create a amqpChannel")

	defer amqpChannel.Close()

	queue, err := amqpChannel.QueueDeclare("add", true, false, false, false, nil)
	handleError(err, "Could not declare `add` queue")

	err = amqpChannel.Qos(1, 0, false)
	handleError(err, "Could not configure QoS")

	messageChannel, err := amqpChannel.Consume(
		queue.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	handleError(err, "Could not register consumer")

	go func() {
		log.Printf("Consumer ready, PID: %d", os.Getpid())
		for d := range messageChannel {
			log.Printf("Received a message: %s", d.Body)

			addTask := &gopher_and_rabbit.AddTask{}

			err := json.Unmarshal(d.Body, addTask)

			if err != nil {
				log.Printf("Error decoding JSON: %s", err)
			}
			errCancel := service.CancelTicketForBonus(addTask.Addr, addTask.Username, addTask.TicketUID)
			for errCancel != nil {
				errCancel = service.CancelTicketForBonus(addTask.Addr, addTask.Username, addTask.TicketUID)
				if errCancel != nil {
					log.Printf("Error again!")
				}

				time.Sleep(10 * time.Second)
			}

			if err := d.Ack(false); err != nil {
				log.Printf("Error acknowledging message : %s", err)
			} else {
				log.Printf("Acknowledged message")
			}

		}
	}()

	port := os.Getenv("PORT")
	r := handlers.Router()
	log.Println("Server is listening on port: ", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
