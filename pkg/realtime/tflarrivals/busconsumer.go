package tflarrivals

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/kr/pretty"
	"github.com/rs/zerolog/log"

	"github.com/adjust/rmq/v5"
)

var totalEvents atomic.Uint64
var successEvents atomic.Uint64

type BusBatchConsumer struct {
}

func NewBusBatchConsumer() *BusBatchConsumer {
	return &BusBatchConsumer{}
}

type BusMonitorEvent struct {
	Line                     string
	DirectionRef             string
	NumberPlate              string
	OriginRef                string
	DestinationRef           string
	OriginAimedDepartureTime string
}

func (c *BusBatchConsumer) Consume(batch rmq.Deliveries) {
	payloads := batch.Payloads()

	for _, payload := range payloads {
		var event BusMonitorEvent
		err := json.Unmarshal([]byte(payload), &event)

		if err != nil {
			continue
		}

		totalEvents.Add(1)

		tripID, err := c.IdentifyBus(event)
		if err == nil {
			successEvents.Add(1)
			log.Info().Str("tripid", tripID).Str("line", event.Line).Msg("Identified")
		} else {
			log.Info().Interface("event", event).Err(err).Msg("Failed to identify")
		}
	}

	log.Info().Float64("rate", float64(successEvents.Load())/float64(totalEvents.Load())).Int("total", int(totalEvents.Load())).Int("success", int(successEvents.Load())).Msg("Events rate")

	if ackErrors := batch.Ack(); len(ackErrors) > 0 {
		for _, err := range ackErrors {
			log.Fatal().Err(err).Msg("Failed to consume from queue")
		}
	}
}

func (c *BusBatchConsumer) IdentifyBus(event BusMonitorEvent) (string, error) {
	eventDirection := strings.ToLower(event.DirectionRef)
	if eventDirection == "1" {
		eventDirection = "inbound"
	} else if eventDirection == "2" {
		eventDirection = "outbound"
	}

	lineTracker := LineArrivalTracker{
		Line: &TfLLine{
			LineID: event.Line,
		},
	}

	arrivalPredictions := lineTracker.GetLatestArrivals()

	// Group all the arrivals predictions that are part of the same journey
	groupedLineArrivals := map[string][]ArrivalPrediction{}
	for _, arrival := range arrivalPredictions {
		if arrival.VehicleID != event.NumberPlate {
			continue
		}

		realtimeJourneyID := fmt.Sprintf(
			"REALTIME:TFL:%s:%s:%s:%s:%s",
			arrival.ModeName,
			arrival.LineID,
			arrival.Direction,
			arrival.VehicleID,
			arrival.DestinationNaptanID,
		)

		groupedLineArrivals[realtimeJourneyID] = append(groupedLineArrivals[realtimeJourneyID], arrival)
	}

	if len(groupedLineArrivals) == 1 {
		for _, lineArrival := range groupedLineArrivals {
			return lineArrival[0].TripID, nil
		}
	}

	return "", errors.New(fmt.Sprintf("Could not be identified. Count: %d", len(groupedLineArrivals)))
}

func (c *BusBatchConsumer) Test() {
	value, err := c.IdentifyBus(BusMonitorEvent{
		Line:                     "269",
		DirectionRef:             "2",
		NumberPlate:              "LJ11ABV",
		OriginRef:                "GB:ATCO:490003975E",
		DestinationRef:           "GB:ATCO:490003975L",
		OriginAimedDepartureTime: "2024-08-23T21:53:00+00:00",
	})

	pretty.Println(value, err)
}