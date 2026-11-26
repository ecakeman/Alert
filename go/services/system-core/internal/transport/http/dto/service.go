package dto

type ServiceSummary struct {
	ServiceID       string  `json:"service_id"`
	ServiceKey      string  `json:"service_key"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	Enabled         bool    `json:"enabled"`
	MonitorStatus   string  `json:"monitor_status"`
	AppStatus       string  `json:"app_status"`
	TargetCount     int     `json:"target_count"`
	LastCheckAt     *string `json:"last_check_at"`
	ActiveEpisodeID *string `json:"active_episode_id"`
}

type ServiceView struct {
	ServiceID              string   `json:"service_id"`
	ServiceKey             string   `json:"service_key"`
	Name                   string   `json:"name"`
	Description            string   `json:"description"`
	Enabled                bool     `json:"enabled"`
	MonitorAggregationMode string   `json:"monitor_aggregation_mode"`
	AuthorityProducerID    string   `json:"authority_producer_id"`
	MonitorStatus          string   `json:"monitor_status"`
	AppStatus              string   `json:"app_status"`
	Targets                []Target `json:"targets"`
	TargetCount            int      `json:"target_count,omitempty"`
	LastCheckAt            *string  `json:"last_check_at,omitempty"`
	ActiveEpisodeID        *string  `json:"active_episode_id,omitempty"`
}

type ServicesPage struct {
	Total    int              `json:"total"`
	Limit    int              `json:"limit"`
	Offset   int              `json:"offset"`
	Services []ServiceSummary `json:"services"`
}

type ServiceCreatePayload struct {
	ServiceKey          string        `json:"service_key"`
	Name                string        `json:"name"`
	Description         string        `json:"description"`
	AuthorityProducerID string        `json:"authority_producer_id"`
	Targets             []TargetInput `json:"targets"`
}

type ServicePatchPayload struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Enabled     *bool   `json:"enabled"`
}
