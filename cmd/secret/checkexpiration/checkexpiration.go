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

// #include "shim.h"
//import "C"
import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientcmd "k8s.io/client-go/tools/clientcmd"
	"opendev.org/airship/airshipctl/pkg/environment"
	"opendev.org/airship/airshipctl/pkg/k8s/client"
)

type secretList []struct {
	secretName string
	secretNs   string
}

type Certificate struct {
	Issuer *Certificate
	ref    interface{}
}

// NewEncryptCommand creates a new command for generating secret information
func NewCheckCommand(rootSettings *environment.AirshipCTLSettings, factory client.Factory) *cobra.Command {

	var duration string
	checkCmd := &cobra.Command{
		Use:   "checkexpiration",
		Short: "Check expiration of TLS Secrets",
		RunE: func(cmd *cobra.Command, args []string) error {

			fmt.Println("Running Check-expiration of all TLS secrets")

			_, err := checkexpiry(rootSettings, factory, duration)
			if err != nil {
				return fmt.Errorf("failed to check expiry: %s", err.Error())
			}
			fmt.Fprint(os.Stdout, "Successfully checked expiry\n")
			return nil

		},
	}

	checkCmd.Flags().StringVar(&duration, "duration", "30",
		"Duration in days. Defaults to 30")
	return checkCmd
}

func checkexpiry(rootSettings *environment.AirshipCTLSettings, factory client.Factory, duration string) ([]secretList, error) {

	d, err := strconv.Atoi(duration)

	if err != nil {
		return nil, err
	}

	kclient, err := factory(rootSettings)
	if err != nil {
		return nil, err
	}

	//	airconfig := rootSettings.Config
	//	_, err := airconfig.GetCurrentContext()
	//	if err != nil {
	//		return nil, err
	//	}

	// Checking the EXPIRY of TLS Secrets
	secretType := "kubernetes.io/tls"
	secrets, err := kclient.ClientSet().CoreV1().Secrets("").List(metav1.ListOptions{FieldSelector: fmt.Sprintf("type=%s", secretType)})

	for _, secret := range secrets.Items {

		fmt.Println("Checking ", secret.Name, "in ", secret.Namespace)

		//		fmt.Println(string(secret.Data["tls.crt"]))

		expiry, err := checkNotAfter([]byte(secret.Data["tls.crt"]), d)
		if err != nil {
			return nil, err
		}

		if expiry {
			fmt.Fprint(os.Stdout, "tls.crt in "+secret.Name+"\n")
		}

		if secret.Data["ca.crt"] != nil {

			expiry, err = checkNotAfter([]byte(secret.Data["ca.crt"]), d)
			if err != nil {
				return nil, err
			}

			if expiry {
				fmt.Fprint(os.Stdout, "ca.crt in "+secret.Name+"\n")
			}

		}

	}

	kSecrets, err := kclient.ClientSet().CoreV1().Secrets("").List(metav1.ListOptions{})

	if err != nil {
		return nil, err
	}

	for _, kSecret := range kSecrets.Items {

		if strings.HasSuffix(kSecret.Name, "-kubeconfig") {

			//			fmt.Println("found ", kSecret.Name)

			for ownerref := range kSecret.OwnerReferences {
				//				fmt.Println("kind " + kSecret.OwnerReferences[ownerref].Kind)

				if kSecret.OwnerReferences[ownerref].Kind == "KubeadmControlPlane" {
					fmt.Println("Checking ", kSecret.Name, " in ", kSecret.Namespace)
					kubecontent, err := clientcmd.Load(kSecret.Data["value"])
					if err != nil {
						return nil, err
					}
					//					fmt.Println(kubecontent.Clusters)

					// Iterate through each Cluster and identify expiry
					for clusterName, clusterData := range kubecontent.Clusters {
						//		fmt.Println(clusterName, string(clusterData.CertificateAuthorityData))
						expiry, err := checkNotAfter(clusterData.CertificateAuthorityData, d)

						if err != nil {
							return nil, err
						}

						if expiry {
							fmt.Fprint(os.Stdout, "CertificateAuthorityData for "+clusterName+" cluster in "+kSecret.Name+"\n")
						}
					}

					// Iterate through each user Certificate and identify expiry
					for userName, userData := range kubecontent.AuthInfos {
						//		fmt.Println(userName, string(userData.ClientCertificateData))

						expiry, err := checkNotAfter(userData.ClientCertificateData, d)

						if err != nil {
							return nil, err
						}

						if expiry {
							fmt.Fprint(os.Stdout, "ClientCertificateData for "+userName+" user in  "+kSecret.Name+"\n")
						}
					}
				}
			}

		}

	}

	return nil, nil
}

func checkNotAfter(certData []byte, d int) (bool, error) {

	block, _ := pem.Decode(certData)

	if block == nil {

		return false, fmt.Errorf("failed to parse certificate PEM")

	}

	cert, err := x509.ParseCertificate(block.Bytes)

	if err != nil {

		return false, fmt.Errorf("failed to parse certificate: " + err.Error())

	}

	//	fmt.Println(cert.NotAfter.Date())
	//	fmt.Println(time.Now().Date())

	return checkIfExpired(cert.NotAfter, d), nil

}

func checkIfExpired(notAfter time.Time, duration int) bool {

	diffTime := notAfter.Sub(time.Now())

	if int(diffTime.Hours()/24) < int(duration) {
		return true
	} else if int(diffTime.Hours()/24) < 0 {
		return true
	}

	return false
}

