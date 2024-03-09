package manager

import (
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/travigo/travigo/pkg/ctdf"
	"github.com/travigo/travigo/pkg/database"
	"github.com/travigo/travigo/pkg/dataimporter/formats"
	"github.com/travigo/travigo/pkg/dataimporter/formats/cif"
	"github.com/travigo/travigo/pkg/dataimporter/formats/gtfs"
	"github.com/travigo/travigo/pkg/dataimporter/formats/naptan"
	"github.com/travigo/travigo/pkg/dataimporter/formats/nationalrailtoc"
	networkrailcorpus "github.com/travigo/travigo/pkg/dataimporter/formats/networkrail-corpus"
	"github.com/travigo/travigo/pkg/dataimporter/formats/siri_vm"
	"github.com/travigo/travigo/pkg/dataimporter/formats/travelinenoc"
	"github.com/travigo/travigo/pkg/redis_client"
	"go.mongodb.org/mongo-driver/bson"
)

func GetDataset(identifier string) (DataSet, error) {
	registered := GetRegisteredDataSets()

	for _, dataset := range registered {
		if dataset.Identifier == identifier {
			log.Info().
				Str("identifier", dataset.Identifier).
				Str("format", string(dataset.Format)).
				Str("provider", dataset.Provider.Name).
				Interface("supports", dataset.SupportedObjects).
				Msg("Found dataset")

			return dataset, nil
		}
	}

	return DataSet{}, errors.New("Dataset could not be found")
}

func (dataset *DataSet) ImportDataset() error {
	var format formats.Format

	switch dataset.Format {
	case DataSetFormatTravelineNOC:
		format = &travelinenoc.TravelineData{}
	case DataSetFormatNaPTAN:
		format = &naptan.NaPTAN{}
	case DataSetFormatNationalRailTOC:
		format = &nationalrailtoc.TrainOperatingCompanyList{}
	case DataSetFormatNetworkRailCorpus:
		format = &networkrailcorpus.Corpus{}
	case DataSetFormatSiriVM:
		format = &siri_vm.SiriVM{}
	case DataSetFormatGTFSSchedule:
		format = &gtfs.Schedule{}
	case DataSetFormatGTFSRealtime:
		format = &gtfs.Realtime{}
	case DataSetFormatCIF:
		format = &cif.CommonInterfaceFormat{}
	default:
		return errors.New(fmt.Sprintf("Unrecognised format %s", dataset.Format))
	}

	if dataset.ImportDestination == ImportDestinationRealtimeQueue {
		if dataset.queue == nil {
			realtimeQueue, err := redis_client.QueueConnection.OpenQueue("realtime-queue")
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to start redis realtime-queue")
			}
			dataset.queue = &realtimeQueue
		}

		var realtimeQueueFormat formats.RealtimeQueueFormat
		realtimeQueueFormat = format.(formats.RealtimeQueueFormat)

		realtimeQueueFormat.SetupRealtimeQueue(*dataset.queue)
	}

	datasource := &ctdf.DataSource{
		OriginalFormat: string(dataset.Format),
		Provider:       dataset.Provider.Name,
		DatasetID:      dataset.Identifier,
		Timestamp:      fmt.Sprintf("%d", time.Now().Unix()),
	}

	source := dataset.Source
	if isValidUrl(dataset.Source) {
		var tempFile *os.File
		tempFile, _ = tempDownloadFile(dataset)

		source = tempFile.Name()
		defer os.Remove(tempFile.Name())
	}

	sourceFileReaders := []io.Reader{}

	file, err := os.Open(source)
	if err != nil {
		return err
	}

	switch dataset.UnpackBundle {
	case BundleFormatNone:
		sourceFileReaders = append(sourceFileReaders, file)
	case BundleFormatGZ:
		gzipDecoder, err := gzip.NewReader(file)
		if err != nil {
			log.Fatal().Err(err).Msg("cannot decode gzip stream")
		}
		defer gzipDecoder.Close()

		sourceFileReaders = append(sourceFileReaders, gzipDecoder)
	case BundleFormatZIP:
		archive, err := zip.OpenReader(source)
		if err != nil {
			panic(err)
		}
		defer archive.Close()

		for _, zipFile := range archive.File {
			zipFileOpen, err := zipFile.Open()
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to open file")
			}
			defer zipFileOpen.Close()

			sourceFileReaders = append(sourceFileReaders, zipFileOpen)
		}
	default:
		return errors.New(fmt.Sprintf("Cannot handle bundle format %s", dataset.UnpackBundle))
	}

	for _, sourceFileReader := range sourceFileReaders {
		err = format.ParseFile(sourceFileReader)
		if err != nil {
			return err
		}

		err = format.Import(
			dataset.Identifier,
			dataset.SupportedObjects,
			datasource,
		)
		if err != nil {
			return err
		}
	}

	if dataset.SupportedObjects.Stops {
		cleanupOldRecords("stops", datasource)
	}
	if dataset.SupportedObjects.StopGroups {
		cleanupOldRecords("stop_groups", datasource)
	}
	if dataset.SupportedObjects.Operators {
		cleanupOldRecords("operators", datasource)
	}
	if dataset.SupportedObjects.OperatorGroups {
		cleanupOldRecords("operator_groups", datasource)
	}
	if dataset.SupportedObjects.Services {
		cleanupOldRecords("services", datasource)
	}
	if dataset.SupportedObjects.Journeys {
		cleanupOldRecords("journeys", datasource)
	}

	return nil
}

func isValidUrl(toTest string) bool {
	_, err := url.ParseRequestURI(toTest)
	if err != nil {
		return false
	}

	u, err := url.Parse(toTest)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	return true
}

func tempDownloadFile(dataset *DataSet) (*os.File, string) {
	req, _ := http.NewRequest("GET", dataset.Source, nil)
	req.Header.Set("user-agent", "curl/7.54.1") // TfL is protected by cloudflare and it gets angry when no user agent is set

	if dataset.DownloadHandler != nil {
		dataset.DownloadHandler(req)
	}

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		log.Fatal().Err(err).Msg("Download file")
	}
	defer resp.Body.Close()

	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	fileExtension := filepath.Ext(dataset.Source)
	if err == nil {
		fileExtension = filepath.Ext(params["filename"])
	}

	tmpFile, err := os.CreateTemp(os.TempDir(), "travigo-data-importer-")
	if err != nil {
		log.Fatal().Err(err).Msg("Cannot create temporary file")
	}

	io.Copy(tmpFile, resp.Body)

	return tmpFile, fileExtension
}

func cleanupOldRecords(collectionName string, datasource *ctdf.DataSource) {
	collection := database.GetCollection(collectionName)

	query := bson.M{
		"$and": bson.A{
			bson.M{"datasource.originalformat": datasource.OriginalFormat},
			bson.M{"datasource.datasetid": datasource.DatasetID},
			bson.M{"datasource.timestamp": bson.M{
				"$ne": datasource.Timestamp,
			}},
		},
	}

	result, _ := collection.DeleteMany(context.Background(), query)

	if result != nil {
		log.Info().
			Str("collection", collectionName).
			Int64("num", result.DeletedCount).
			Msg("Cleaned up old records")
	}
}
