package tutils

import (
	"context"
	"os"
	"strings"
	"time"

	aiscos "github.com/NVIDIA/aistore/cmn/cos"
	aisclient "github.com/ais-operator/pkg/client"
	corev1 "k8s.io/api/core/v1"
)

const (
	EnvNodeImage           = "AIS_TEST_NODE_IMAGE"
	EnvPrevNodeImage       = "AIS_TEST_PREV_NODE_IMAGE"
	EnvInitImage           = "AIS_TEST_INIT_IMAGE"
	EnvPrevInitImage       = "AIS_TEST_PREV_INIT_IMAGE"
	EnvAPIMode             = "AIS_TEST_API_MODE"
	EnvTestStorageClass    = "TEST_STORAGECLASS"
	EnvTestStorageHostPath = "TEST_STORAGE_HOSTPATH"
	EnvTestEphemeral       = "TEST_EPHEMERAL_CLUSTER"
	GKEDefaultStorageClass = "standard"

	K8sProviderGKE      = "gke"
	K8sProviderMinikube = "minikube"
	K8sProviderUnknown  = "unknown"
)

type AISTestContext struct {
	K8sProvider       string
	StorageClass      string
	StorageHostPath   string
	NodeImage         string
	InitImage         string
	PreviousNodeImage string
	PreviousInitImage string
	LogsImage         string
	Ephemeral         bool
	APIMode           string
}

func NewAISTestContext(ctx context.Context, k8sClient *aisclient.K8sClient) (*AISTestContext, error) {
	k8sProvider, err := initK8sProvider(ctx, k8sClient)
	if err != nil {
		return nil, err
	}
	return &AISTestContext{
		K8sProvider:       k8sProvider,
		StorageClass:      initStorageClass(k8sClient, k8sProvider),
		StorageHostPath:   initStorageHostPath(),
		NodeImage:         initNodeImage(),
		InitImage:         initInitImage(),
		PreviousNodeImage: initPrevNodeImage(),
		PreviousInitImage: initPrevInitImage(),
		LogsImage:         DefaultLogsImage,
		Ephemeral:         initEphemeral(),
		APIMode:           initAPIMode(),
	}, nil
}

func initK8sProvider(ctx context.Context, client *aisclient.K8sClient) (string, error) {
	nodes := &corev1.NodeList{}
	err := client.List(ctx, nodes)
	if err != nil {
		return "", err
	}
	for i := range nodes.Items {
		if strings.Contains(nodes.Items[i].Name, "gke") {
			return K8sProviderGKE, nil
		}
	}
	return K8sProviderUnknown, nil
}

func initEphemeral() bool {
	ephemeral, _ := aiscos.ParseBool(os.Getenv(EnvTestEphemeral))
	return ephemeral
}

func initPrevInitImage() string {
	return getOrDefaultEnv(EnvPrevInitImage, DefaultPrevInitImage)
}

func initInitImage() string {
	return getOrDefaultEnv(EnvInitImage, DefaultInitImage)
}

func initNodeImage() string {
	return getOrDefaultEnv(EnvNodeImage, DefaultNodeImage)
}

func initPrevNodeImage() string {
	if os.Getenv(EnvNodeImage) != "" {
		return getOrDefaultEnv(EnvPrevNodeImage, DefaultNodeImage)
	}
	return getOrDefaultEnv(EnvPrevNodeImage, DefaultPrevNodeImage)
}

func initStorageClass(k8sClient *aisclient.K8sClient, k8sProvider string) string {
	storageClass := os.Getenv(EnvTestStorageClass)
	if storageClass == "" && k8sProvider == K8sProviderGKE {
		storageClass = GKEDefaultStorageClass
	} else if storageClass == "" {
		storageClass = "ais-operator-test-storage"
		CreateAISStorageClass(context.Background(), k8sClient, storageClass)
	}
	return storageClass
}

func initStorageHostPath() string {
	return getOrDefaultEnv(EnvTestStorageHostPath, "/etc/ais/"+strings.ToLower(aiscos.CryptoRandS(6)))
}

func initAPIMode() string {
	return os.Getenv("AIS_TEST_API_MODE")
}

func getOrDefaultEnv(envVar, defaultVal string) string {
	val := strings.TrimSpace(os.Getenv(envVar))
	if val != "" {
		return val
	}
	return defaultVal
}

func (c *AISTestContext) GetClusterCreateTimeout() time.Duration {
	if c.K8sProvider == K8sProviderGKE {
		return 5 * time.Minute
	}
	return 3 * time.Minute
}

func (c *AISTestContext) GetClusterCreateLongTimeout() time.Duration {
	if c.K8sProvider == K8sProviderGKE {
		return 8 * time.Minute
	}
	return 6 * time.Minute
}

func (c *AISTestContext) GetLBExistenceTimeout() (timeout, interval time.Duration) {
	if c.K8sProvider == K8sProviderGKE {
		return 4 * time.Minute, 5 * time.Second
	}
	return 10 * time.Second, 200 * time.Millisecond
}
