package darwin

import (
	"encoding/xml"
	"io"

	"github.com/rs/zerolog/log"
	"golang.org/x/net/html/charset"
)

func ParseXMLFile(reader io.Reader) (PushPortData, error) {
	pushPortData := PushPortData{}

	d := xml.NewDecoder(reader)
	d.CharsetReader = charset.NewReaderLabel
	for {
		tok, err := d.Token()
		if tok == nil || err == io.EOF {
			// EOF means we're done.
			break
		} else if err != nil {
			log.Fatal().Msgf("Error decoding token: %s", err)
			return pushPortData, err
		}

		switch ty := tok.(type) {
		case xml.StartElement:
			if ty.Name.Local == "TS" {
				var trainStatus TrainStatus

				if err = d.DecodeElement(&trainStatus, &ty); err != nil {
					log.Fatal().Msgf("Error decoding item: %s", err)
				} else {
					pushPortData.TrainStatuses = append(pushPortData.TrainStatuses, trainStatus)
				}
			} else if ty.Name.Local == "schedule" {
				var schedule Schedule

				if err = d.DecodeElement(&schedule, &ty); err != nil {
					log.Fatal().Msgf("Error decoding item: %s", err)
				} else {
					pushPortData.Schedules = append(pushPortData.Schedules, schedule)
				}
			} else if ty.Name.Local == "formationLoading" {
				var formationLoading FormationLoading

				if err = d.DecodeElement(&formationLoading, &ty); err != nil {
					log.Fatal().Msgf("Error decoding item: %s", err)
				} else {
					pushPortData.FormationLoadings = append(pushPortData.FormationLoadings, formationLoading)
				}
			}
		}
	}

	return pushPortData, nil
}
