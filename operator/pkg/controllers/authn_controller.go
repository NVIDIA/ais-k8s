// Package controllers contains k8s controller logic for AIS cluster
/*
* Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.
 */
package controllers

import (
	"fmt"
	"time"

	"github.com/NVIDIA/aistore/api/authn"
	aisv1 "github.com/ais-operator/api/v1beta1"
)

type authNConfig struct {
	adminUser string
	adminPass string
	port      string
	host      string
	protocol  string
}

// AuthN constants
const (
	AuthNServiceHostName = "ais-authn.ais"
	AuthNServicePort     = "52001"
	AuthNAdminUser       = "admin"
	AuthNAdminPass       = "admin"

	AuthNServiceHostVar = "AIS_AUTHN_SERVICE_HOST"
	AuthNServicePortVar = "AIS_AUTHN_SERVICE_PORT"
)

// getAdminToken retrieves an admin token from AuthN service for the given AIS cluster.
func (r *AIStoreReconciler) getAdminToken(ais *aisv1.AIStore) (string, error) {
	if ais.Spec.AuthNSecretName == nil {
		return "", nil
	}

	authNURL := fmt.Sprintf("%s://%s:%s", r.authN.protocol, r.authN.host, r.authN.port)
	authNBP := _baseParams(authNURL, "")
	zeroDuration := time.Duration(0)

	tokenMsg, err := authn.LoginUser(*authNBP, r.authN.adminUser, r.authN.adminPass, &zeroDuration)
	if err != nil {
		return "", fmt.Errorf("failed to login admin user to AuthN: %w", err)
	}

	r.log.Info("Successfully logged in as Admin to AuthN")
	return tokenMsg.Token, nil
}
