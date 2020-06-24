/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cluster

import (
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"opendev.org/airship/airshipctl/pkg/cluster"
	"opendev.org/airship/airshipctl/pkg/environment"
	"opendev.org/airship/airshipctl/pkg/k8s/client"
)

const (
	renewCertLong = `
Renews control plane certificates if the expiration threshold is met followed by restarting control plane components.
`

	renewCertExample = `
  airshipctl cluster renew-certs
`

	successMessage = `
Successfully triggered rolling update to renew certificates for control plane node using clusterapi.

Run the following commands to monitor the rolling update.

Monitor machine provisioning:
  kubectl get machines -w

Monitor control plane status:
  kubectl get kcp -w

Monitor the health of kubernetes control plane nodes
  kubectl get nodes -w
`
)

// NewRenewCertsCommand creates a command to renew control plane certificates for the target cluster
func NewRenewCertsCommand(rootSettings *environment.AirshipCTLSettings, factory client.Factory) *cobra.Command {
	moveCmd := &cobra.Command{
		Use:     "renew-certs",
		Short:   "Renew control plane certificates close to expiration and restart control plane components",
		Long:    renewCertLong[1:],
		Example: renewCertExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			stdout := cmd.OutOrStdout()
			err := renewCerts(rootSettings, factory, stdout)
			if err != nil {
				return fmt.Errorf("failed renewing certificates: %s", err.Error())
			}
			fmt.Fprint(stdout, fmt.Sprintf("%s", successMessage))
			return nil
		},
	}

	return moveCmd
}

// renewCerts rotates control plane certificates in the target cluster
func renewCerts(rootSettings *environment.AirshipCTLSettings, factory client.Factory, writer io.Writer) error {
	return cluster.RollingUpdateControlPlane(rootSettings, factory, writer)
}
