package utils

import "os"

const (
	EnvApp        = "APP_ENV"
	EnvProduction = "production"
)

func IsProductionEnv() bool {
	return os.Getenv(EnvApp) == EnvProduction
}
