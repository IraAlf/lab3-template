package service

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"time"

	"lab2/src/gateway-service/internal/gopher_and_rabbit"
	"lab2/src/gateway-service/internal/models"

	"github.com/streadway/amqp"
)

var N = 0

func handleError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}

}

func CalncelTicketController(ticketServiceAddress, bonusServiceAddress, username string, ticketUID string) error {
	_, err := CancelTicket(ticketServiceAddress, username, ticketUID)
	if err != nil {
		return err
	}

	err = CancelTicketForBonus(bonusServiceAddress, username, ticketUID)
	if err != nil {
		conn, err := amqp.Dial(gopher_and_rabbit.Config.AMQPConnectionURL)
		handleError(err, "Can't connect to AMQP")
		defer conn.Close()

		amqpChannel, err := conn.Channel()
		handleError(err, "Can't create a amqpChannel")

		defer amqpChannel.Close()

		queue, err := amqpChannel.QueueDeclare("add", true, false, false, false, nil)
		handleError(err, "Could not declare `add` queue")

		rand.Seed(time.Now().UnixNano())

		addTask := gopher_and_rabbit.AddTask{Addr: bonusServiceAddress, Username: username, TicketUID: ticketUID}
		body, err := json.Marshal(addTask)
		if err != nil {
			handleError(err, "Error encoding JSON")
		}

		err = amqpChannel.Publish("", queue.Name, false, false, amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "text/plain",
			Body:         body,
		})

		if err != nil {
			log.Fatalf("Error publishing message: %s", err)
		}
	}

	return nil
}

func CheckFlight(flightaddr string) chan bool {
	ticker := time.NewTicker(5 * time.Second)

	stopChan := make(chan bool)
	go func(ticker *time.Ticker) {

		err := ManageFlight(flightaddr)
		if err == nil {
			N = 0
			ticker.Stop()
		}
	}(ticker)

	return stopChan
}

func UserTicketsController(ticketServiceAddress, flightServiceAddress, username string) (*[]models.TicketInfo, error) {
	tickets, err := GetUserTickets(ticketServiceAddress, username)
	if err != nil {
		return nil, fmt.Errorf("Failed to get user tickets: %s\n", err)
	}

	ticketsInfo := make([]models.TicketInfo, 0)
	for _, ticket := range *tickets {
		ticketInfo := models.TicketInfo{
			TicketUID:    ticket.TicketUID,
			FlightNumber: ticket.FlightNumber,
			FromAirport:  fmt.Sprintf("%s %s", "None", "None"),
			ToAirport:    fmt.Sprintf("%s %s", "None", "None"),
			Date:         "01-01-1970",
			Price:        ticket.Price,
			Status:       ticket.Status,
		}

		err := fmt.Errorf("error")

		for err != nil && N < 10 {
			flight, err := GetFlight(flightServiceAddress, ticket.FlightNumber)
			if err != nil {
				ticketsInfo = append(ticketsInfo, ticketInfo)
				N += 1
				continue
			}

			airportFrom, err := GetAirport(flightServiceAddress, flight.FromAirportId)
			if err != nil {
				ticketsInfo = append(ticketsInfo, ticketInfo)
				N += 1
				continue
			}
			airportTo, err := GetAirport(flightServiceAddress, flight.ToAirportId)
			if err != nil {
				ticketsInfo = append(ticketsInfo, ticketInfo)
				N += 1
				continue
			}
			err = nil
			ticketInfo.Date = flight.Date
			ticketInfo.ToAirport = fmt.Sprintf("%s %s", airportTo.City, airportTo.Name)
			ticketInfo.FromAirport = fmt.Sprintf("%s %s", airportFrom.City, airportFrom.Name)
		}
		if err != nil {
			CheckFlight(flightServiceAddress)
		}
		ticketsInfo = append(ticketsInfo, ticketInfo)
	}

	return &ticketsInfo, nil
}

func UserInfoController(ticketServiceAddress, flightServiceAddress, bonusServiceAddress, username string) (*models.UserInfo, error) {
	ticketsInfo, err := UserTicketsController(ticketServiceAddress, flightServiceAddress, username)
	if err != nil {
		return nil, fmt.Errorf("Failed to get user tickets: %s", err)
	}

	privilege, err := GetPrivilegeShortInfo(bonusServiceAddress, username)

	if err == nil {

		userInfo := &models.UserInfo{
			TicketsInfo: ticketsInfo,
			Privilege: &models.PrivilegeShortInfo{
				Status:  privilege.Status,
				Balance: privilege.Balance,
			},
		}
		return userInfo, nil
	} else {
		userInfo := &models.UserInfo{
			TicketsInfo: ticketsInfo,
			Privilege: &models.PrivilegeShortInfo{
				Status:  "",
				Balance: 0,
			},
		}
		return userInfo, fmt.Errorf("100")
	}

}

func UserPrivilegeController(bonusServiceAddress, username string) (*models.PrivilegeInfo, error) {
	privilegeShortInfo, err := GetPrivilegeShortInfo(bonusServiceAddress, username)
	if err != nil {
		return nil, fmt.Errorf("Failed to get user tickets: %s", err)
	}

	privilegeHistory, err := GetPrivilegeHistory(bonusServiceAddress, privilegeShortInfo.ID)
	if err != nil {
		return nil, fmt.Errorf("Failed to get privilege info: %s", err)
	}

	privilegeInfo := &models.PrivilegeInfo{
		Status:  privilegeShortInfo.Status,
		Balance: privilegeShortInfo.Balance,
		History: privilegeHistory,
	}

	return privilegeInfo, nil
}

func BuyTicketController(tAddr, fAddr, bAddr, username string, info *models.BuyTicketInfo) (*models.PurchaseTicketInfo, error) {
	flight, err := GetFlight(fAddr, info.FlightNumber)
	if err != nil {
		return nil, fmt.Errorf("Failed to get flights: %s", err)
	}

	airportFrom, err := GetAirport(fAddr, flight.FromAirportId)
	if err != nil {
		return nil, fmt.Errorf("Failed to get airport: %s", err)
	}

	airportTo, err := GetAirport(fAddr, flight.ToAirportId)
	if err != nil {
		return nil, fmt.Errorf("Failed to get airport: %s", err)
	}

	moneyPaid := flight.Price
	bonusesPaid := 0
	diff := int(float32(info.Price) * 0.1)
	optype := "FILL_IN_BALANCE"

	if info.PaidFromBalance {
		if info.Price > 0 {
			bonusesPaid = 0
		} else {
			bonusesPaid = info.Price
		}

		moneyPaid = moneyPaid - bonusesPaid
		diff = -bonusesPaid
		optype = "DEBIT_THE_ACCOUNT"
	}

	uid, err := CreateTicket(tAddr, username, info.FlightNumber, flight.Price)
	if err != nil {
		return nil, fmt.Errorf("Failed to create ticket: %s", err)
	}

	if !info.PaidFromBalance {
		if err := CreatePrivilege(bAddr, username, diff); err != nil {
			DeleteTicket(tAddr, uid)
			return nil, fmt.Errorf("Failed to get privilege info: %s", err)
		}
	}

	err = CreatePrivilegeHistoryRecord(bAddr, uid, flight.Date, optype, 1, diff)
	if err != nil {
		//return nil, fmt.Errorf("Failed to create bonus history record: %s", err)
	}

	purchaseInfo := models.PurchaseTicketInfo{
		TicketUID:     uid,
		FlightNumber:  info.FlightNumber,
		FromAirport:   fmt.Sprintf("%s %s", airportFrom.City, airportFrom.Name),
		ToAirport:     fmt.Sprintf("%s %s", airportTo.City, airportTo.Name),
		Date:          flight.Date,
		Price:         flight.Price,
		PaidByMoney:   moneyPaid,
		PaidByBonuses: bonusesPaid,
		Status:        "PAID",
		Privilege: &models.PrivilegeShortInfo{
			Balance: diff,
			Status:  "GOLD",
		},
	}

	return &purchaseInfo, nil
}
