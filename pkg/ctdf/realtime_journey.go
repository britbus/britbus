package ctdf

import (
	"time"
)

var RealtimeJourneyIDFormat = "REALTIME:%s:%s"

type RealtimeJourney struct {
	PrimaryIdentifier string `groups:"basic"`

	//JourneyRef string   `groups:"internal"`
	Journey *Journey `groups:"basic"`

	CreationDateTime     time.Time `groups:"detailed"`
	ModificationDateTime time.Time `groups:"detailed"`

	DataSource *DataSource `groups:"internal"`

	VehicleLocation Location `groups:"basic"`
	VehicleBearing  float64  `groups:"basic"`

	DepartedStopRef string `groups:"basic"`
	DepartedStop    *Stop  `groups:"basic" bson:"-"`

	NextStopRef string `groups:"basic"`
	NextStop    *Stop  `groups:"basic" bson:"-"`

	NextStopArrival   time.Time `groups:"basic"`
	NextStopDeparture time.Time `groups:"basic"`

	Stops  map[string]*RealtimeJourneyStops `groups:"basic"` // Historic & future estimates
	Offset time.Duration

	Reliability RealtimeJourneyReliabilityType `groups:"basic"`

	VehicleRef string `groups:"internal"`
}

type RealtimeJourneyReliabilityType string

const (
	RealtimeJourneyReliabilityExternalProvided     RealtimeJourneyReliabilityType = "ExternalProvided"
	RealtimeJourneyReliabilityLocationWithTrack                                   = "LocationWithTrack"
	RealtimeJourneyReliabilityLocationWithoutTrack                                = "LocationWithoutTrack"
)

func (r *RealtimeJourney) IsActive() bool {
	timedOut := (time.Now().Sub(r.ModificationDateTime)).Minutes() > 10

	if timedOut {
		return false
	}

	// If tis still nil then give up
	if r.Journey == nil {
		return false
	}

	return true // TODO REMOVE THIS OBVIOUSLY YOU IDIOT

	lastPathItem := r.Journey.Path[len(r.Journey.Path)-1]

	if lastPathItem.DestinationStop == nil {
		lastPathItem.GetDestinationStop()

		// If we still cant find it then mark as in-active
		if lastPathItem.DestinationStop == nil {
			return false
		}
	}

	now := time.Now()
	lastPathItemArrivalDateless := lastPathItem.DestinationArrivalTime
	lastPathItemArrival := time.Date(
		now.Year(), now.Month(), now.Day(), lastPathItemArrivalDateless.Hour(), lastPathItemArrivalDateless.Minute(), lastPathItemArrivalDateless.Second(), lastPathItemArrivalDateless.Nanosecond(), now.Location(),
	)
	timeFromlastPathItemArrival := lastPathItemArrival.Sub(now).Minutes()

	distanceEndStopLocation := r.VehicleLocation.Distance(lastPathItem.DestinationStop.Location)

	// If we're past the last path item arrival time & vehicle location is less than 150m from it then class journey as in-active
	return !((timeFromlastPathItemArrival < 0) && (distanceEndStopLocation < 150))
}

type RealtimeJourneyStops struct {
	StopRef string `groups:"basic"`
	Stop    *Stop  `groups:"basic" bson:"-"`

	ArrivalTime   time.Time `groups:"basic"`
	DepartureTime time.Time `groups:"basic"`

	TimeType RealtimeJourneyStopTimeType `groups:"basic"`
}

type RealtimeJourneyStopTimeType string

const (
	// Unknown         RealtimeJourneyStopTimeType = "Unknown"
	RealtimeJourneyStopTimeHistorical      RealtimeJourneyStopTimeType = "Historical"
	RealtimeJourneyStopTimeEstimatedFuture                             = "EstimatedFuture"
)

func GetActiveRealtimeJourneyCutOffDate() time.Time {
	return time.Now().Add(-10 * time.Minute)
}
