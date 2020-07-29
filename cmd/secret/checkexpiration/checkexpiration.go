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
	"os"

	"github.com/spf13/cobra"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"opendev.org/airship/airshipctl/pkg/environment"
	"opendev.org/airship/airshipctl/pkg/k8s/client"
)

type secretList []struct {
	secretName string
	secretNs   string
}

// NewEncryptCommand creates a new command for generating secret information
func NewCheckCommand(rootSettings *environment.AirshipCTLSettings, factory client.Factory) *cobra.Command {

	checkCmd := &cobra.Command{
		Use:   "checkexpiration",
		Short: "Check expiration of TLS Secrets",
		RunE: func(cmd *cobra.Command, args []string) error {

			fmt.Println("Running Check-expiration of all TLS secrets")

			_, err := checkexpiry(rootSettings, factory)
			if err != nil {
				return fmt.Errorf("failed to check expiry: %s", err.Error())
			}
			fmt.Fprint(os.Stdout, "Successfully checked expiry\n")
			return nil

		},
	}
	return checkCmd
}

func checkexpiry(rootSettings *environment.AirshipCTLSettings, factory client.Factory) ([]secretList, error) {
	kclient, err := factory(rootSettings)
	if err != nil {
		return nil, err
	}

//	airconfig := rootSettings.Config
//	_, err := airconfig.GetCurrentContext()
//	if err != nil {
//		return nil, err
//	}

  secretType := "kubernetes.io/tls"
	secrets, err := kclient.ClientSet().CoreV1().Secrets("").List(metav1.ListOptions{FieldSelector: fmt.Sprintf("type=%s", secretType)})

	for _, secret := range secrets.Items {

		fmt.Println(secret.Name,  secret.Namespace)
				fmt.Println(string(secret.Data["tls.crt"]))
	}

	return nil, nil
}

