package services

import (
	"github.com/NVIDIA/aistore/api"
	"github.com/NVIDIA/aistore/api/apc"
	"github.com/NVIDIA/aistore/cmn"
	"github.com/NVIDIA/aistore/cmn/cos"
	"github.com/NVIDIA/aistore/core/meta"
	aisv1 "github.com/ais-operator/api/v1beta1"
)

type (
	AIStoreClientInterface interface {
		DecommissionCluster(rmUserData bool) error
		DecommissionNode(actValue *apc.ActValRmNode) (xid string, err error)
		GetClusterMap() (smap *meta.Smap, err error)
		Health(isPrimary bool) error
		SetClusterConfigUsingMsg(configToUpdate *cmn.ConfigToSet, transient bool) error
		SetPrimaryProxy(newPrimaryID string, force bool) error
		ShutdownCluster() error
		GetAuthToken() string
		HasValidBaseParams(ais *aisv1.AIStore) bool
	}

	AIStoreClient struct {
		params *api.BaseParams
	}
)

func (c *AIStoreClient) GetAuthToken() string {
	if c.params == nil {
		return ""
	}
	return c.params.Token
}

// HasValidBaseParams checks if the client has valid params for the given AIS cluster configuration
func (c *AIStoreClient) HasValidBaseParams(ais *aisv1.AIStore) bool {
	if c.params == nil {
		return false
	}
	// Determine whether HTTPS should be used based on the presence of a TLS secret / TLS issuer and
	// verify if the URL's protocol matches the expected protocol (HTTPS or HTTP)
	httpsCheck := cos.IsHTTPS(c.params.URL) == ais.UseHTTPS()

	// Check if the token and AuthN secret are correctly aligned:
	// - Valid if both are either set or both are unset
	authNCheck := (c.params.Token == "" && ais.Spec.AuthNSecretName == nil) ||
		(c.params.Token != "" && ais.Spec.AuthNSecretName != nil)

	return httpsCheck && authNCheck
}

func (c *AIStoreClient) DecommissionCluster(rmUserData bool) error {
	return api.DecommissionCluster(*c.params, rmUserData)
}

func (c *AIStoreClient) DecommissionNode(actValue *apc.ActValRmNode) (string, error) {
	return api.DecommissionNode(*c.params, actValue)
}

func (c *AIStoreClient) GetClusterMap() (smap *meta.Smap, err error) {
	return api.GetClusterMap(*c.params)
}

func (c *AIStoreClient) Health(isPrimary bool) error {
	return api.Health(*c.params, isPrimary /*readyToRebalance*/)
}

func (c *AIStoreClient) SetClusterConfigUsingMsg(config *cmn.ConfigToSet, transient bool) error {
	return api.SetClusterConfigUsingMsg(*c.params, config, transient)
}

func (c *AIStoreClient) SetPrimaryProxy(newPrimaryID string, force bool) error {
	return api.SetPrimaryProxy(*c.params, newPrimaryID, force)
}

func (c *AIStoreClient) ShutdownCluster() error {
	return api.ShutdownCluster(*c.params)
}

func NewAIStoreClient(params *api.BaseParams) *AIStoreClient {
	return &AIStoreClient{
		params: params,
	}
}
