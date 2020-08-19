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

package checkexpiration

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"opendev.org/airship/airshipctl/pkg/environment"
	"opendev.org/airship/airshipctl/pkg/k8s/client"
	"opendev.org/airship/airshipctl/pkg/secret/checkexpiration"
)

// NewCheckCommand creates a new command for generating secret information
func NewCheckCommand(rootSettings *environment.AirshipCTLSettings, factory client.Factory) *cobra.Command {

	var duration string
	var contentType string
	checkCmd := &cobra.Command{
		Use:   "checkexpiration",
		Short: "Check expiration of TLS Secrets",
		RunE: func(cmd *cobra.Command, args []string) error {

			if !(strings.ToLower(contentType) == "yaml" || strings.ToLower(contentType) == "json") {
				return fmt.Errorf("Only YAML and JSON are allowed")
			}

			//			fmt.Println("Running Check-expiration of all TLS secrets")

			err := checkexpiration.CheckexpiryData(rootSettings, factory, duration, contentType)

			if err != nil {
				return fmt.Errorf("failed to check expiry: %s", err.Error())
			}
			//			fmt.Fprint(os.Stdout, "Successfully checked expiry\n")
			return nil

		},
	}

	checkCmd.Flags().StringVar(&duration, "duration", "30",
		"Duration in days. Defaults to 30")
	checkCmd.Flags().StringVarP(&contentType, "output", "o", "", "convert output to yaml or json")

	checkCmd.MarkFlagRequired("output")

	return checkCmd
}
