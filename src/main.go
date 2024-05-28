package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/gorilla/mux"
)

type ECSService struct {
	Name string
	IP   string
}

type Config struct {
	AWSRegion         string
	ECSCluster        string
	ProxyPort         string
	HeaderRoutingName string
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	} else if value == "" && defaultValue == "" {
		panic(fmt.Errorf("missing mandatory env %s", key))
	}
	return defaultValue
}

func LoadConfig() Config {
	return Config{
		AWSRegion:         getEnv("AWS_REGION", "us-west-2"),
		ECSCluster:        getEnv("ECS_CLUSTER", ""),
		ProxyPort:         getEnv("PROXY_PORT", "8080"),
		HeaderRoutingName: getEnv("DEFAULT_ORG_ID", "X-Org-ID"),
	}
}

func main() {
	config := LoadConfig()

	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String(config.AWSRegion),
	}))
	log.Printf("AWS Region: %s cluster %s", config.AWSRegion, config.ECSCluster)

	ecsClient := ecs.New(sess)
	cluster := config.ECSCluster

	serviceDetails := buildServiceDetails(ecsClient, cluster)
	log.Printf("service details %v", serviceDetails)

	r := mux.NewRouter()
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		orgID := r.Header.Get(config.HeaderRoutingName)
		if orgID == "" {
			err := fmt.Errorf("missing required header %s", config.HeaderRoutingName)
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

		serviceIP, ok := getServiceDetail(orgID, serviceDetails)
		if !ok {
			// try sync with the cluster
			serviceDetails = buildServiceDetails(ecsClient, cluster)
			serviceIP, ok = getServiceDetail(orgID, serviceDetails)
			if !ok {
				http.Error(w, "Service not found for Org-ID", http.StatusNotFound)
				return
			}
		}

		http.Redirect(w, r, fmt.Sprintf("http://%s", serviceIP), http.StatusTemporaryRedirect)
	})

	http.ListenAndServe(":"+config.ProxyPort, r)
}

func listServices(ecsClient *ecs.ECS, cluster string) ([]*string, error) {
	resp, err := ecsClient.ListServices(&ecs.ListServicesInput{
		Cluster: aws.String(cluster),
	})
	if err != nil {
		return nil, err
	}
	return resp.ServiceArns, nil
}

func listTasks(ecsClient *ecs.ECS, cluster string) ([]*string, error) {
	var tasks []*string
	resp, err := ecsClient.ListTasks(&ecs.ListTasksInput{
		Cluster: aws.String(cluster),
	})
	if err != nil {
		return nil, err
	}
	tasks = append(tasks, resp.TaskArns...)
	return tasks, nil
}

func buildServiceDetails(ecsClient *ecs.ECS, cluster string) []ECSService {
	services, err := listServices(ecsClient, cluster)
	if err != nil {
		log.Fatalf("Failed to list services: %v", err)
	}
	log.Printf("services %v", services)

	tasks, err := listTasks(ecsClient, cluster)
	if err != nil {
		log.Fatalf("Failed to list tasks: %v", err)
	}
	log.Printf("tasks %v", tasks)

	return getServiceDetails(ecsClient, cluster, tasks)
}
func getServiceDetails(ecsClient *ecs.ECS, cluster string, tasks []*string) []ECSService {
	serviceDetails := []ECSService{}
	for _, taskArn := range tasks {
		taskDetail, err := ecsClient.DescribeTasks(&ecs.DescribeTasksInput{
			Cluster: aws.String(cluster),
			Tasks:   []*string{taskArn},
		})
		if err != nil {
			log.Printf("Failed to describe task: %v", err)
			continue
		}

		for _, task := range taskDetail.Tasks {
			for _, container := range task.Containers {
				for _, network := range container.NetworkInterfaces {
					serviceDetails = append(serviceDetails, ECSService{
						Name: *container.Name,
						IP:   *network.PrivateIpv4Address,
					})
				}
			}
		}
	}
	return serviceDetails
}

func getServiceDetail(orgID string, services []ECSService) (string, bool) {
	for _, svc := range services {
		if strings.Contains(svc.Name, orgID) {
			return svc.IP, true
		}
	}
	log.Printf("Service not found for Org-ID %s", orgID)
	return "", false
}
