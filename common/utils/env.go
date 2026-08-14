package utils

import (
	"os"

	"meta-api/common/env"
)

func IsProductionEnv() bool {
	return os.Getenv(env.AppEnv) == env.AppProduction
}
