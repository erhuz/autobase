package storage

import (
	"net"
	"time"
)

type CloudProvider struct {
	Code          string
	Description   string
	ProviderImage string
}

type CloudRegion struct {
	ProviderCode string
	RegionGroup  string
	RegionName   string
	Description  string
}

type CloudInstance struct {
	ProviderCode  string
	InstanceGroup string
	InstanceName  string
	Arch          string
	Cpu           int64
	Ram           int64
	PriceHourly   float64
	PriceMonthly  float64
	Currency      string
	UpdatedAt     time.Time
	SharedCpu     bool
}

type CloudImage struct {
	ProviderCode string
	Region       string
	Image        interface{}
	Arch         string
	OsName       string
	OsVersion    string
	UpdatedAt    time.Time
}

type CloudVolume struct {
	ProviderCode      string
	VolumeType        string
	VolumeDescription string
	VolumeMinSize     int64
	VolumeMaxSize     int64
	PriceMonthly      float64
	Currency          string
	IsDefault         bool
	UpdatedAt         time.Time
}

type CloudProviderInfo struct {
	Code           string
	CloudRegions   []CloudRegion
	CloudInstances []CloudInstance
	CloudVolumes   []CloudVolume
	CloudImages    []CloudImage
}

type PostgresVersion struct {
	MajorVersion int64
	ReleaseDate  time.Time
	EndOfLife    time.Time
}

type Setting struct {
	ID        int64
	Name      string
	Value     interface{}
	CreatedAt time.Time
	UpdatedAt *time.Time
}

type GetSettingsReq struct {
	Name *string

	Limit  *int64
	Offset *int64
}

type MetaPagination struct {
	Limit  int64
	Offset int64
	Count  int64
}

type Project struct {
	ID          int64
	Name        string
	Description *string
	CreatedAt   time.Time
	UpdatedAt   *time.Time
}

type Environment struct {
	ID          int64
	Name        string
	Description *string
	CreatedAt   time.Time
	UpdatedAt   *time.Time
}

type AddEnvironmentReq struct {
	Name        string
	Description string
}

type SecretView struct {
	ProjectID      int64
	ID             int64
	Name           string
	Type           string
	CreatedAt      time.Time
	UpdatedAt      *time.Time
	IsUsed         bool
	UsedByClusters *string
}

type GetSecretsReq struct {
	ProjectID int64
	Name      *string
	Type      *string
	SortBy    *string

	Limit  *int64
	Offset *int64
}

type AddSecretReq struct {
	ProjectID int64
	Type      string
	Name      string
	Value     []byte
	SecretKey string
}

type EditSecretReq struct {
	ProjectID int64
	Type      *string
	Name      *string
	Value     []byte
	SecretKey string
}

type Extension struct {
	Name               string
	Description        *string
	Url                *string
	Image              *string
	PostgresMinVersion *string
	PostgresMaxVersion *string
	Contrib            bool
}

type GetExtensionsReq struct {
	Type            *string
	PostgresVersion *string

	Limit  *int64
	Offset *int64
}

type Cluster struct {
	ID                    int64
	ProjectID             int64
	EnvironmentID         int64
	SecretID              *int64
	Name                  string
	Status                string
	Description           string
	Location              *string
	ConnectionInfo        interface{}
	ExtraVars             []byte
	Inventory             []byte
	ServersCount          int32
	PostgreVersion        int32
	CreatedAt             time.Time
	UpdatedAt             *time.Time
	DeletedAt             *time.Time
	Flags                 uint32
	QueryAnalyticsManaged bool
	QueryAnalyticsDesired bool
}

type GetClustersReq struct {
	ProjectID       int64
	Name            *string
	SortBy          *string
	Status          *string
	Location        *string
	ServerCount     *int64
	PostgresVersion *int64
	EnvironmentID   *int64
	CreatedAtFrom   *time.Time
	CreatedAtTo     *time.Time

	Limit  *int64
	Offset *int64
}

type CreateClusterReq struct {
	ProjectID             int64
	EnvironmentID         int64
	Name                  string
	Description           string
	SecretID              *int64
	ExtraVars             []byte
	Location              string
	ServerCount           int
	PostgreSqlVersion     int
	Status                string
	Inventory             []byte
	QueryAnalyticsManaged *bool
	QueryAnalyticsDesired *bool
}

type UpdateClusterReq struct {
	ID             int64
	ConnectionInfo interface{}
	Status         *string
	Flags          *uint32
}

type Operation struct {
	ID                int64
	ProjectID         int64
	ClusterID         int64
	DockerCode        string
	Cid               string
	Type              string
	Status            string
	Log               *string
	CreatedAt         time.Time
	UpdatedAt         *time.Time
	Actor             string
	SanitizedParams   []byte
	PreflightSnapshot []byte
	Plan              []byte
	AffectedNodes     []byte
	FinalVerification []byte
	SafeNextAction    *string
}

type OperationView struct {
	ProjectID   int64
	ClusterID   int64
	ID          int64
	Started     time.Time
	Finished    *time.Time
	Type        string
	Status      string
	Cluster     string
	Environment string
}

type CreateOperationReq struct {
	ProjectID         int64
	ClusterID         int64
	DockerCode        string
	Type              string
	Cid               string
	Actor             string
	SanitizedParams   []byte
	PreflightSnapshot []byte
	Plan              []byte
	AffectedNodes     []byte
}

type UpdateOperationReq struct {
	ID                int64
	Status            *string
	Logs              *string
	DockerCode        *string
	FinalVerification []byte
	SafeNextAction    *string
}

type OperationPreflight struct {
	ID            int64
	ClusterID     int64
	Type          string
	Observed      []byte
	Desired       []byte
	Checks        []byte
	Blockers      []byte
	Plan          []byte
	AffectedNodes []byte
	Confirmation  string
	TopologyHash  string
	ExpiresAt     time.Time
	ConsumedAt    *time.Time
	CreatedAt     time.Time
}

type CreateOperationPreflightReq struct {
	ClusterID     int64
	Type          string
	Observed      []byte
	Desired       []byte
	Checks        []byte
	Blockers      []byte
	Plan          []byte
	AffectedNodes []byte
	Confirmation  string
	TopologyHash  string
	ExpiresAt     time.Time
}

type GetOperationsReq struct {
	ProjectID   int64
	StartedFrom time.Time
	EndedTill   time.Time
	ClusterName *string
	Type        *string
	Status      *string
	Environment *string
	SortBy      *string

	Limit  *int64
	Offset *int64
}

type Server struct {
	ID             int64
	ClusterID      int64
	Name           string
	Location       *string
	Role           string
	Status         string
	IpAddress      net.IP
	Timeline       *int64
	Lag            *int64
	Tags           interface{}
	PendingRestart *bool
	CreatedAt      time.Time
	UpdatedAt      *time.Time
}

type CreateServerReq struct {
	ClusterID      int64
	ServerName     string
	ServerLocation *string
	IpAddress      string
}

type UpdateServerReq struct {
	ClusterID int64
	IpAddress string

	Name           string
	Role           *string
	Status         *string
	Timeline       *int64
	Lag            *int64
	Tags           interface{}
	PendingRestart *bool
}

type QueryAnalyticsSource struct {
	ClusterID        int64
	ServerID         int64
	NodeBootTime     *time.Time
	Status           string
	ExtensionVersion *string
	LastBucketStart  *time.Time
	LastCollectedAt  *time.Time
	LastErrorCode    *string
}

type QueryAnalyticsBucket struct {
	ClusterID         int64
	ServerID          int64
	NodeBootTime      time.Time
	BucketID          int64
	BucketStart       time.Time
	BucketEnd         time.Time
	Complete          bool
	Calls             int64
	TotalExecTimeMs   float64
	MaxExecTimeMs     float64
	Rows              int64
	SharedBlocksHit   int64
	SharedBlocksRead  int64
	TempBlocksRead    int64
	TempBlocksWritten int64
	ReadTimeMs        float64
	WriteTimeMs       float64
	WALBytes          int64
	Samples           []QueryAnalyticsSample
}

type QueryAnalyticsSample struct {
	ClusterID         int64
	ServerID          int64
	NodeBootTime      time.Time
	BucketID          int64
	FingerprintID     string
	NormalizedQuery   string
	DatabaseName      string
	RoleName          string
	ApplicationName   string
	Calls             int64
	TotalExecTimeMs   float64
	MinExecTimeMs     float64
	MaxExecTimeMs     float64
	MeanExecTimeMs    float64
	Rows              int64
	SharedBlocksHit   int64
	SharedBlocksRead  int64
	TempBlocksRead    int64
	TempBlocksWritten int64
	ReadTimeMs        float64
	WriteTimeMs       float64
	WALBytes          int64
	LatencyHistogram  []string
	TopTotalTime      bool
	TopMaxLatency     bool
}

type QueryAnalyticsFilter struct {
	From            time.Time
	To              time.Time
	ServerID        *int64
	DatabaseName    *string
	RoleName        *string
	ApplicationName *string
}

type QueryAnalyticsStatus struct {
	State              string
	Managed            bool
	Desired            bool
	PostgresVersion    int32
	ExpectedNodeCount  int64
	CollectedNodeCount int64
}

type QueryAnalyticsCoverage struct {
	ServerID         int64
	ServerName       string
	ServerRole       string
	ServerStatus     string
	CollectionStatus string
	ExtensionVersion *string
	LastBucketStart  *time.Time
	LastCollectedAt  *time.Time
	LastErrorCode    *string
}

type QueryAnalyticsMetrics struct {
	Calls             int64
	TotalExecTimeMs   float64
	MaxExecTimeMs     float64
	MeanExecTimeMs    float64
	Rows              int64
	SharedBlocksHit   int64
	SharedBlocksRead  int64
	TempBlocksRead    int64
	TempBlocksWritten int64
	ReadTimeMs        float64
	WriteTimeMs       float64
	WALBytes          int64
}

type QueryAnalyticsSeriesPoint struct {
	BucketStart time.Time
	QueryAnalyticsMetrics
}

type QueryAnalyticsQuery struct {
	FingerprintID   string
	NormalizedQuery string
	QueryAnalyticsMetrics
	TopTotalTime  bool
	TopMaxLatency bool
}

type QueryAnalyticsFilterOptions struct {
	Databases    []string
	Roles        []string
	Applications []string
}

type QueryAnalyticsOverview struct {
	Status   QueryAnalyticsStatus
	Coverage []QueryAnalyticsCoverage
	Summary  QueryAnalyticsMetrics
	Series   []QueryAnalyticsSeriesPoint
	Queries  []QueryAnalyticsQuery
	Filters  QueryAnalyticsFilterOptions
}

type QueryAnalyticsDetail struct {
	Fingerprint QueryAnalyticsQuery
	Series      []QueryAnalyticsSeriesPoint
	Histogram   []string
}
