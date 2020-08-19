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

package resetSaToken

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"opendev.org/airship/airshipctl/pkg/environment"
	"opendev.org/airship/airshipctl/pkg/k8s/client"
)

var secretType = "kubernetes.io/service-account-token"

// NewResetCommand creates a new command for generating secret information
func NewResetCommand(rootSettings *environment.AirshipCTLSettings, factory client.Factory) *cobra.Command {
	var namespace, secretName string
	resetCmd := &cobra.Command{
		Use:   "rotate-sa-token",
		Short: "Rotate tokens of Service Accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := rotateToken(rootSettings, factory, namespace, secretName)
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

// rotateToken - rotates the token 1. Deletes the secret and 2. Deletes its pod
func rotateToken(rootSettings *environment.AirshipCTLSettings, factory client.Factory, ns string, secretName string) error {
	kclient, err := factory(rootSettings)
	if err != nil {
		return err
	}

	if secretName == "" {

		secrets, err := kclient.ClientSet().CoreV1().Secrets(ns).List(metav1.ListOptions{FieldSelector: fmt.Sprintf("type=%s", secretType)})

		if len(secrets.Items) < 1 {
			return fmt.Errorf("No SA tokens found/Invalid namespace")
		}

		for _, secret := range secrets.Items {
			fmt.Fprint(os.Stdout, "Rotating token - "+secret.Name+"\n")
			err = deleteSecret(kclient, secret.Name, ns)
			if err != nil {
				return err
			}

			err = deletePod(kclient, secret.Name, ns)
			if err != nil {
				return err
			}

		}
	} else {

		secret, err := kclient.ClientSet().CoreV1().Secrets(ns).Get(secretName, metav1.GetOptions{})
		if err != nil {
			return err
		}

		if string(secret.Type) == secretType {
			fmt.Fprint(os.Stdout, "Rotating token - "+secretName+"\n")
			err := deleteSecret(kclient, secretName, ns)
			if err != nil {
				return err
			}

			err = deletePod(kclient, secretName, ns)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf(secretName + " is not a Service Account Token")
		}
	}
	return nil
}

// deleteSecret- deletes the secret
func deleteSecret(kclient client.Interface, secretName string, ns string) error {
	deleteOptions := &metav1.DeleteOptions{}
	var zero int64 = 0
	deleteOptions.GracePeriodSeconds = &zero

	err := kclient.ClientSet().CoreV1().Secrets(ns).Delete(secretName, deleteOptions)
	if err != nil {
		return err
	}
	return nil
}

// deletePod - identifies the secret relationship with pods and deletes corresponding pods
func deletePod(kclient client.Interface, secretName string, ns string) error {
	pods, err := kclient.ClientSet().CoreV1().Pods(ns).List(metav1.ListOptions{})
	deleteOptions := &metav1.DeleteOptions{}
	var zero int64 = 0
	deleteOptions.GracePeriodSeconds = &zero
	for _, pod := range pods.Items {

		for volume := range pod.Spec.Volumes {

			if pod.Spec.Volumes[volume].Name == secretName {
				fmt.Fprint(os.Stdout, "Deleting pod - "+pod.Name+"\n")
				err1 := kclient.ClientSet().CoreV1().Pods(ns).Delete(pod.Name, deleteOptions)
				if err1 != nil {
					return err
				}
			}

		}
	}
	return nil
}
