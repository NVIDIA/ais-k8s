package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	aisapc "github.com/NVIDIA/aistore/api/apc"
	aiscmn "github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	opcmn "github.com/ais-operator/pkg/resources/cmn"
	jsoniter "github.com/json-iterator/go"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

const (
	defaultClusterDomain = "cluster.local"
	retryInterval        = 10 * time.Second
)

func getRequiredEnv(envVar string) string {
	val := strings.TrimSpace(os.Getenv(envVar))
	failOnCondition(val != "", "env %q is required!", envVar)
	return val
}

func getOrDefaultEnv(envVar, defaultVal string) string {
	val := strings.TrimSpace(os.Getenv(envVar))
	if val != "" {
		return val
	}
	return defaultVal
}

func failOnCondition(cond bool, msg string, a ...any) {
	if cond {
		return
	}
	klog.Errorf(msg, a...)
	os.Exit(1)
}

func failOnError(err error) {
	if err == nil {
		return
	}
	klog.Error(err)
	os.Exit(1)
}

func fetchExternalIP(svcName, namespace string) string {
	config, err := rest.InClusterConfig()
	failOnError(err)
	client := kubernetes.NewForConfigOrDie(config)
	for i := 1; i <= 6; i++ {
		externalService, err := client.CoreV1().Services(namespace).Get(context.TODO(), svcName, metav1.GetOptions{})
		if err != nil && errors.IsNotFound(err) {
			failOnError(err)
		}

		if err == nil && len(externalService.Status.LoadBalancer.Ingress) > 0 {
			return externalService.Status.LoadBalancer.Ingress[0].IP
		}
		klog.Warningf("couldn't fetch valid external loadbalancer IP for svc %q, attempt: %d", svcName, i)
		time.Sleep(retryInterval)
	}
	failOnCondition(false, "Failed to fetch external IP for target")
	return ""
}

func getMappedHostname(original, mapPath string) string {
	data, err := os.ReadFile(mapPath)
	failOnError(err)
	var hostmap map[string]string
	err = jsoniter.Unmarshal(data, &hostmap)
	failOnError(err)
	if mapped, ok := hostmap[original]; ok && strings.TrimSpace(mapped) != "" {
		return mapped
	}
	return original
}

func main() {
	var (
		role                   string
		aisLocalConfigTemplate string
		outputLocalConfig      string
		hostnameMapFile        string

		localConf aiscmn.LocalConfig
	)

	flag.StringVar(&role, "role", "", "AISNode role")
	flag.StringVar(&aisLocalConfigTemplate, "local_config_template", "", "local template file path")
	flag.StringVar(&outputLocalConfig, "output_local_config", "", "output local config path")
	flag.StringVar(&hostnameMapFile, "hostname_map_file", "", "path to file containing hostname map")
	flag.Parse()

	failOnCondition(role == aisapc.Proxy || role == aisapc.Target, "invalid role provided %q", role)

	confBytes, err := os.ReadFile(aisLocalConfigTemplate)
	failOnError(err)
	err = json.Unmarshal(confBytes, &localConf)
	failOnError(err)

	namespace := getRequiredEnv(opcmn.EnvNS)
	serviceName := getRequiredEnv(opcmn.EnvServiceName)
	podName := getRequiredEnv(opcmn.EnvPodName)
	clusterDomain := getOrDefaultEnv(opcmn.EnvClusterDomain, defaultClusterDomain)
	publicHostName := getOrDefaultEnv(opcmn.EnvPublicHostname, "")
	podDNS := fmt.Sprintf("%s.%s.%s.svc.%s", podName, serviceName, namespace, clusterDomain)

	localConf.HostNet.HostnameIntraControl = podDNS
	localConf.HostNet.HostnameIntraData = podDNS
	localConf.HostNet.Hostname = publicHostName

	if role == aisapc.Target {
		useHostNetwork, err := cos.ParseBool(getOrDefaultEnv(opcmn.EnvHostNetwork, "false"))
		failOnError(err)
		if useHostNetwork {
			localConf.HostNet.HostnameIntraData = ""
			localConf.HostNet.Hostname = ""
		}

		useExternalLB, err := cos.ParseBool(getOrDefaultEnv(opcmn.EnvEnableExternalAccess, "false"))
		failOnError(err)
		if useExternalLB {
			localConf.HostNet.Hostname = fetchExternalIP(podName, namespace)
		}
	}

	if hostnameMapFile != "" {
		localConf.HostNet.Hostname = getMappedHostname(localConf.HostNet.Hostname, hostnameMapFile)
	}

	data, err := jsoniter.Marshal(localConf)
	failOnError(err)
	err = os.WriteFile(outputLocalConfig, data, 0o644)
	failOnError(err)
}
