module github.com/ecakeman/alert/services/episode-svc

go 1.27.4

require (
	github.com/ecakeman/alert/services/contracts v0.0.0
	github.com/ecakeman/alert/services/platform v0.0.0
	github.com/gin-gonic/gin v1.12.0
	github.com/jackc/pgx/v5 v5.13.0
)

replace (
	github.com/ecakeman/alert/services/contracts => ../contracts
	github.com/ecakeman/alert/services/platform => ../platform
)