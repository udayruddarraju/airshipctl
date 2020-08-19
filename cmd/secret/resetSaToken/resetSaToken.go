/*
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     https://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package resetsatoken

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"opendev.org/airship/airshipctl/pkg/environment"
	"opendev.org/airship/airshipctl/pkg/k8s/client"
	resetsatoken "opendev.org/airship/airshipctl/pkg/secret/resetsatoken"
)

var secretType = "kubernetes.io/service-account-token"

// NewResetCommand creates a new command for generating secret information
func NewResetCommand(rootSettings *environment.AirshipCTLSettings, factory client.Factory) *cobra.Command {
	var namespace, secretName string
	resetCmd := &cobra.Command{
		Use:   "rotate-sa-token",
		Short: "Rotate tokens of Service Accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := resetsatoken.RotateToken(rootSettings, factory, namespace, secretName)
			if err != nil {
				return fmt.Errorf("failed to rotate token: %s", err.Error())
			}
			fmt.Fprint(os.Stdout, "Successfully rotated token\n")
			return nil

		},
	}

	resetCmd.Flags().StringVar(&namespace, "namespace", "",
		"namespace of the Service Account Token")
	resetCmd.Flags().StringVar(&secretName, "secret-name", "",
		"name of the secret containing Service Account Token")

	resetCmd.MarkFlagRequired("namespace")

	return resetCmd
}
