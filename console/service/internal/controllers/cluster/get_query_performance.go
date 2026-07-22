package cluster

import (
	"errors"
	"fmt"
	"postgresql-cluster-console/internal/controllers"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/models"
	clusterapi "postgresql-cluster-console/restapi/operations/cluster"
	"strconv"
	"time"

	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
)

type getQueryPerformanceHandler struct{ db storage.IStorage }

func NewGetQueryPerformanceHandler(db storage.IStorage) clusterapi.GetClustersIDQueryPerformanceHandler {
	return &getQueryPerformanceHandler{db: db}
}

func NewGetQueryPerformanceDetailHandler(db storage.IStorage) clusterapi.GetClustersIDQueryPerformanceFingerprintIDHandler {
	return &getQueryPerformanceDetailHandler{&getQueryPerformanceHandler{db: db}}
}

func (h *getQueryPerformanceHandler) Handle(param clusterapi.GetClustersIDQueryPerformanceParams) middleware.Responder {
	filter, err := queryPerformanceFilter(param.From, param.To, param.ServerID, param.Database, param.Role, param.Application)
	if err != nil {
		return clusterapi.NewGetClustersIDQueryPerformanceBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	overview, err := h.db.GetQueryAnalyticsOverview(param.HTTPRequest.Context(), param.ID, filter)
	if err != nil {
		return clusterapi.NewGetClustersIDQueryPerformanceBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	return clusterapi.NewGetClustersIDQueryPerformanceOK().WithPayload(queryPerformanceOverviewModel(overview))
}

func (h *getQueryPerformanceHandler) HandleDetail(param clusterapi.GetClustersIDQueryPerformanceFingerprintIDParams) middleware.Responder {
	filter, err := queryPerformanceFilter(param.From, param.To, param.ServerID, param.Database, param.Role, param.Application)
	if err != nil {
		return clusterapi.NewGetClustersIDQueryPerformanceFingerprintIDBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	detail, err := h.db.GetQueryAnalyticsDetail(param.HTTPRequest.Context(), param.ID, param.FingerprintID, filter)
	if err != nil {
		return clusterapi.NewGetClustersIDQueryPerformanceFingerprintIDBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	if detail == nil {
		return clusterapi.NewGetClustersIDQueryPerformanceFingerprintIDBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("query fingerprint not found"), controllers.BaseError))
	}
	histogram := make([]int64, len(detail.Histogram))
	for i, value := range detail.Histogram {
		histogram[i], err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			return clusterapi.NewGetClustersIDQueryPerformanceFingerprintIDBadRequest().WithPayload(controllers.MakeErrorPayload(fmt.Errorf("invalid stored histogram: %w", err), controllers.BaseError))
		}
	}
	return clusterapi.NewGetClustersIDQueryPerformanceFingerprintIDOK().WithPayload(&models.ResponseQueryPerformanceDetail{
		Fingerprint: queryPerformanceQueryModel(detail.Fingerprint),
		Series:      queryPerformanceSeriesModels(detail.Series),
		Histogram:   histogram,
	})
}

type getQueryPerformanceDetailHandler struct{ *getQueryPerformanceHandler }

func (h *getQueryPerformanceDetailHandler) Handle(param clusterapi.GetClustersIDQueryPerformanceFingerprintIDParams) middleware.Responder {
	return h.HandleDetail(param)
}

func queryPerformanceFilter(from, to *strfmt.DateTime, serverID *int64, database, role, application *string) (*storage.QueryAnalyticsFilter, error) {
	now := time.Now().UTC()
	end := now
	if to != nil {
		end = time.Time(*to)
		if end.After(now) {
			end = now
		}
	}
	start := end.Add(-time.Hour)
	if from != nil {
		start = time.Time(*from)
	}
	if cutoff := now.Add(-7 * 24 * time.Hour); start.Before(cutoff) {
		start = cutoff
	}
	if !start.Before(end) {
		return nil, errors.New("from must be before to")
	}
	if serverID != nil && *serverID < 1 {
		return nil, errors.New("server_id must be positive")
	}
	return &storage.QueryAnalyticsFilter{
		From: start, To: end, ServerID: serverID,
		DatabaseName: database, RoleName: role, ApplicationName: application,
	}, nil
}

func queryPerformanceOverviewModel(value *storage.QueryAnalyticsOverview) *models.ResponseQueryPerformance {
	coverage := make([]*models.QueryPerformanceCoverage, len(value.Coverage))
	for i, source := range value.Coverage {
		coverage[i] = &models.QueryPerformanceCoverage{
			ServerID: source.ServerID, ServerName: source.ServerName, ServerRole: source.ServerRole,
			ServerStatus: source.ServerStatus, CollectionStatus: source.CollectionStatus,
			ExtensionVersion: source.ExtensionVersion, LastBucketStart: queryPerformanceTime(source.LastBucketStart),
			LastCollectedAt: queryPerformanceTime(source.LastCollectedAt), LastErrorCode: source.LastErrorCode,
		}
	}
	queries := make([]*models.QueryPerformanceQuery, len(value.Queries))
	for i, query := range value.Queries {
		queries[i] = queryPerformanceQueryModel(query)
	}
	return &models.ResponseQueryPerformance{
		Status: &models.QueryPerformanceStatus{
			State: value.Status.State, Managed: value.Status.Managed, Desired: value.Status.Desired,
			PostgresVersion: int64(value.Status.PostgresVersion), ExpectedNodeCount: value.Status.ExpectedNodeCount,
			CollectedNodeCount: value.Status.CollectedNodeCount,
		},
		Coverage: coverage, Summary: queryPerformanceMetricsModel(value.Summary),
		Series: queryPerformanceSeriesModels(value.Series), Queries: queries,
		Filters: &models.QueryPerformanceFilters{
			Databases: value.Filters.Databases, Roles: value.Filters.Roles, Applications: value.Filters.Applications,
		},
	}
}

func queryPerformanceQueryModel(value storage.QueryAnalyticsQuery) *models.QueryPerformanceQuery {
	return &models.QueryPerformanceQuery{
		FingerprintID: value.FingerprintID, NormalizedQuery: value.NormalizedQuery,
		Metrics:      queryPerformanceMetricsModel(value.QueryAnalyticsMetrics),
		TopTotalTime: value.TopTotalTime, TopMaxLatency: value.TopMaxLatency,
	}
}

func queryPerformanceSeriesModels(values []storage.QueryAnalyticsSeriesPoint) []*models.QueryPerformancePoint {
	result := make([]*models.QueryPerformancePoint, len(values))
	for i, point := range values {
		result[i] = &models.QueryPerformancePoint{
			BucketStart: strfmt.DateTime(point.BucketStart), Metrics: queryPerformanceMetricsModel(point.QueryAnalyticsMetrics),
		}
	}
	return result
}

func queryPerformanceMetricsModel(value storage.QueryAnalyticsMetrics) *models.QueryPerformanceMetrics {
	return &models.QueryPerformanceMetrics{
		Calls: value.Calls, TotalExecTimeMs: value.TotalExecTimeMs, MaxExecTimeMs: value.MaxExecTimeMs,
		MeanExecTimeMs: value.MeanExecTimeMs, Rows: value.Rows, SharedBlocksHit: value.SharedBlocksHit,
		SharedBlocksRead: value.SharedBlocksRead, TempBlocksRead: value.TempBlocksRead,
		TempBlocksWritten: value.TempBlocksWritten, ReadTimeMs: value.ReadTimeMs,
		WriteTimeMs: value.WriteTimeMs, WalBytes: value.WALBytes,
	}
}

func queryPerformanceTime(value *time.Time) *strfmt.DateTime {
	if value == nil {
		return nil
	}
	result := strfmt.DateTime(*value)
	return &result
}
