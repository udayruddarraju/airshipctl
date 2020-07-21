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

package decrypt

import (
	"github.com/spf13/cobra"

	"opendev.org/airship/airshipctl/pkg/environment"
	"opendev.org/airship/airshipctl/pkg/k8s/client"
	"opendev.org/airship/airshipctl/pkg/secret/sops"
)

// NewDecryptCommand creates a new command for generating secret information
func NewDecryptCommand(rootSettings *environment.AirshipCTLSettings, factory client.Factory) *cobra.Command {
	var fromFile, toFile string

	decryptCmd := &cobra.Command{
		Use:   "decrypt",
		Short: "Decrypt a Kubernetes secret yaml file",
		RunE: func(cmd *cobra.Command, args []string) error {
			kclient, err := factory(rootSettings)
			if err != nil {
				return err
			}

			airconfig := rootSettings.Config
			context, err := airconfig.GetCurrentContext()
			if err != nil {
				return err
			}
			sopsClient := sops.NewClient(kclient, context.ClusterName(), context.KubeContext().Namespace)
			if err = sopsClient.InitializeKeys(); err != nil {
				return err
			}

			_, err = sopsClient.Decrypt(fromFile, toFile)
			return err
		},
	}
	decryptCmd.Flags().StringVar(&fromFile, "from-file", "",
		"Path to the secret yaml file in sops encrypted format that is to be decrypted.")
	decryptCmd.Flags().StringVar(&toFile, "to-file", "",
		"Path to the new secret yaml file that will be in the plaintext format.")
	decryptCmd.MarkFlagRequired("from-file")
	decryptCmd.MarkFlagRequired("to-file")

	return decryptCmd
}
