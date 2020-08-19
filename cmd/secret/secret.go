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

package secret

import (
	"github.com/spf13/cobra"
	"opendev.org/airship/airshipctl/cmd/secret/checkexpiration"
	"opendev.org/airship/airshipctl/cmd/secret/generate"
	"opendev.org/airship/airshipctl/cmd/secret/resetSaToken"
	"opendev.org/airship/airshipctl/pkg/environment"
	"opendev.org/airship/airshipctl/pkg/k8s/client"
)

// NewSecretCommand creates a new command for managing airshipctl secrets
func NewSecretCommand(rootSettings *environment.AirshipCTLSettings) *cobra.Command {
	secretRootCmd := &cobra.Command{
		Use: "secret",
		// TODO(howell): Make this more expressive
		Short: "Manage secrets",
	}

	rootSettings.InitConfig()

	secretRootCmd.AddCommand(generate.NewGenerateCommand())
	secretRootCmd.AddCommand(checkexpiration.NewCheckCommand(rootSettings, client.DefaultClient))
	secretRootCmd.AddCommand(resetsatoken.NewResetCommand(rootSettings, client.DefaultClient))

	return secretRootCmd
}
