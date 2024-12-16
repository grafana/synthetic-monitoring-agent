package screenshot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	logproto "github.com/grafana/loki/pkg/push"
	"github.com/grafana/synthetic-monitoring-agent/internal/model"
	"github.com/grafana/synthetic-monitoring-agent/internal/pusher"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Url struct {
	Name         string `json:"name"`
	PresignedURL string `json:"pre_signed_url"`
	Method       string `json:"method"`
}

type PresignedUrlRequest struct {
	Service   string `json:"service"`
	Operation string `json:"operation"`
	Files     []Url  `json:"files"`
}

type PresignedUrlResponse struct {
	Service   string `json:"service"`
	Operation string `json:"operation"`
	Files     []Url  `json:"urls"`
}

type ScreenshotHandler struct {
	logger    zerolog.Logger
	Publisher pusher.Publisher
}

func New(logger zerolog.Logger, publisher pusher.Publisher) *ScreenshotHandler {
	return &ScreenshotHandler{logger: logger, Publisher: publisher}
}

func (s ScreenshotHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	tenantId, err := strconv.ParseInt(r.Header.Get("tenantId"), 10, 64)
	if err != nil {
		http.Error(w, "Missing tenant ID", http.StatusInternalServerError)
	}

	s.logger.Warn().Str("tenantId", r.Header.Get("tenantId")).Msg("headers")
	var request PresignedUrlRequest

	// FIXME
	endpoint := "minio:9000"
	accessKey := "minioadmin"
	secretKey := "minioadmin"
	bucketName := "screenshots"

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: false,
	})
	if err != nil {
		http.Error(w, "Failed to create object store client", http.StatusInternalServerError)
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, "Failed to decode JSON", http.StatusBadRequest)
		log.Printf("Error decoding JSON: %v", err)
		return
	}

	response := PresignedUrlResponse{
		Service:   "aws_s3",
		Operation: "upload",
	}

	for _, file := range request.Files {
		putExpiry := time.Minute * 15
		getExpiry := time.Hour * 24
		ctx := context.Background()

		prefix := "screenshots/1/" + time.Now().String() + "/"

		putUrl, err := minioClient.PresignedPutObject(ctx, bucketName, prefix+file.Name, putExpiry)
		if err != nil {
			http.Error(w, "Failed to create presigned URL", http.StatusInternalServerError)
		}
		response.Files = append(response.Files, Url{Name: file.Name, PresignedURL: putUrl.String(), Method: http.MethodPut})

		getUrl, err := minioClient.PresignedGetObject(ctx, bucketName, file.Name, getExpiry, nil)
		if err != nil {
			http.Error(w, "Failed to create presigned URL", http.StatusInternalServerError)
		}

		s.logger.Info().Str("url", getUrl.String()).Msg("browser screenshot")

		buf := &bytes.Buffer{}
		targetLogger := zerolog.New(buf)
		targetLogger.Info().Str("url", getUrl.String()).Msg("browser screenshot")
		s.Publisher.Publish(screenshotData{
			tenantId: model.GlobalID(tenantId),
			streams: Streams{
				{
					Labels: fmt.Sprintf(`{instance="test", job="test"}`), // FIXME
					Entries: []logproto.Entry{
						{
							Timestamp: time.Now(),
							Line:      fmt.Sprintf(`{"level"="info", url":%q, "message":"browser screenshot"}`, getUrl.String()), // FIXME,
						},
					},
				},
			},
		})

		s.logger.Info().Str("buf", buf.String()).Msg("buf")
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		log.Printf("Error encoding response: %v", err)
	}
}

type (
	Streams = []logproto.Stream
)

type screenshotData struct {
	tenantId model.GlobalID
	streams  Streams
}

func (d screenshotData) Metrics() []prompb.TimeSeries {
	return nil
}

func (d screenshotData) Streams() Streams {
	return d.streams
}

func (d screenshotData) Tenant() model.GlobalID {
	return d.tenantId
}
