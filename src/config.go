package main

import (
	"fmt"
	"os"
)

type Config struct {
	AWSRegion         string
	ECSCluster        string
	ProxyPort         string
	HeaderRoutingName string
}

func LoadConfig() Config {
	return Config{
		AWSRegion:         getEnv("AWS_REGION", "us-west-2"),
		ECSCluster:        getEnv("ECS_CLUSTER", ""),
		ProxyPort:         getEnv("PROXY_PORT", "8080"),
		HeaderRoutingName: getEnv("DEFAULT_ORG_ID", "X-Org-ID"),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	} else if value == "" && defaultValue == "" {
		fmt.Errorf("missing mandatory env %s", key)
	}
	return defaultValue
}
