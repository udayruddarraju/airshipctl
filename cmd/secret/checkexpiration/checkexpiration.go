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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

var doOnce sync.Once

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

	//      airconfig := rootSettings.Config
	//      _, err := airconfig.GetCurrentContext()
	//      if err != nil {
	//              return nil, err
	//      }

	flagY := false
	y := tabwriter.NewWriter(os.Stdout, 50, 8, 1, ' ', 0)
	fmt.Fprintln(y, "")
	fmt.Fprintln(y, "===========\t==========\t===========")
	fmt.Fprintln(y, "CERTIFICATE\tSECRET NAME\tEXPIRY DATE")
	fmt.Fprintln(y, "===========\t==========\t===========")

	// Checking the EXPIRY of TLS Secrets
	secretType := "kubernetes.io/tls"
	secrets, err := kclient.ClientSet().CoreV1().Secrets("").List(metav1.ListOptions{FieldSelector: fmt.Sprintf("type=%s", secretType)})

	for _, secret := range secrets.Items {

		//		fmt.Println("Checking ", secret.Name, "in ", secret.Namespace)

		expiry, notAfter, err := checkNotAfter([]byte(secret.Data["tls.crt"]), d)
		if err != nil {
			return nil, err
		}

		if expiry {

			sY := fmt.Sprintf("%s\t%s\t[%s]", "tls.crt", secret.Name, notAfter)
			fmt.Fprintln(y, sY)

			flagY = true
			//			fmt.Fprint(os.Stdout, "tls.crt in "+secret.Name+" expires on "+notAfter+"\n")
		}

		if secret.Data["ca.crt"] != nil {

			expiry, notAfter, err = checkNotAfter([]byte(secret.Data["ca.crt"]), d)
			if err != nil {
				return nil, err
			}

			if expiry {
				sY := fmt.Sprintf("%s\t%s\t[%s]", "ca.crt", secret.Name, notAfter)
				fmt.Fprintln(y, sY)

				flagY = true
				//				fmt.Fprint(os.Stdout, "ca.crt in "+secret.Name+" expires on "+notAfter+"\n")
			}

		}

		if flagY {
			y.Flush()
		}

	}

	kSecrets, err := kclient.ClientSet().CoreV1().Secrets("").List(metav1.ListOptions{})

	if err != nil {
		return nil, err
	}

	for _, kSecret := range kSecrets.Items {

		if strings.HasSuffix(kSecret.Name, "-kubeconfig") {

			for ownerref := range kSecret.OwnerReferences {
				if kSecret.OwnerReferences[ownerref].Kind == "KubeadmControlPlane" {
					//					fmt.Println("Checking ", kSecret.Name, " in ", kSecret.Namespace)
					kubecontent, err := clientcmd.Load(kSecret.Data["value"])
					if err != nil {
						return nil, err
					}

					// Iterate through each Cluster and identify expiry
					for clusterName, clusterData := range kubecontent.Clusters {
						expiry, notAfter, err := checkNotAfter(clusterData.CertificateAuthorityData, d)

						if err != nil {
							return nil, err
						}

						if expiry {

							kHeader()

							fmt.Fprint(os.Stdout, "CertificateAuthorityData for "+clusterName+" cluster in "+kSecret.Name+" expires on ["+notAfter+"]\n")

							//							fmt.Fprint(os.Stdout, "CertificateAuthorityData for "+clusterName+" cluster in "+kSecret.Name+" expires on "+notAfter+"\n")
						}
					}

					// Iterate through each user Certificate and identify expiry
					for userName, userData := range kubecontent.AuthInfos {

						expiry, notAfter, err := checkNotAfter(userData.ClientCertificateData, d)

						if err != nil {
							return nil, err
						}

						if expiry {
							kHeader()
							fmt.Fprint(os.Stdout, "ClientCertificateData for "+userName+" user in  "+kSecret.Name+" expires on ["+notAfter+"]\n")
						}
					}
				}
			}

		}

	}

	//  Checking the expiry of Workload nodes certificates

	// GVR for the HostConfig Operator (Ansible Operator)
	gvr := schema.GroupVersionResource{
		Group:    "hostconfig.airshipit.org",
		Version:  "v1alpha1",
		Resource: "hostconfigs",
	}

	// Here the CRD HostConfig is running on the Workload Cluster which keeps the expiry information handy and the below code just reads the information, parses it and reports back
	hcList, err := kclient.DynamicClient().Resource(gvr).List(metav1.ListOptions{})

	if err != nil {
		if err.Error() == "the server could not find the requested resource" {
			return nil, nil
		} else {
			return nil, err
		}
	}

	for _, hc := range hcList.Items {

		fld, exists, err := unstructured.NestedMap(hc.Object, "status", "hostConfigStatus")

		if err != nil {
			return nil, err

		}
		if !exists {
			fmt.Println("Doesnot exist")
		}

		fmt.Print("Checking the Expiry on the Workload Nodes")

		w := tabwriter.NewWriter(os.Stdout, 2, 6, 3, ' ', 0)
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "===========\t========\t===========")
		fmt.Fprintln(w, "CERTIFICATE\tHOSTNAME\tEXPIRY DATE")
		fmt.Fprintln(w, "===========\t========\t===========")

		flag := false

		for hostName, value := range fld {

			for k, v := range value.(map[string]interface{}) {

				if k == "Execute kubeadm command cert expirataion" {
					if v.(map[string]interface{})["status"] == "Successful" {
						for _, list := range v.(map[string]interface{})["results"].([]interface{}) {

							if list.(map[string]interface{})["status"] == "Successful" {

								data := fmt.Sprintf("%v", list.(map[string]interface{})["stdout"])
								// Below code block does string manipulation. Needs to be changed if the `kubeadm alpha certs check-expiration` structure changes
								// This is the expected Structure

								// CERTIFICATE                EXPIRES                  RESIDUAL TIME   CERTIFICATE AUTHORITY   EXTERNALLY MANAGED
								// admin.conf                 Aug 10, 2021 13:25 UTC   364d                                    no
								// apiserver                  Aug 10, 2021 13:25 UTC   364d            ca                      no
								// apiserver-etcd-client      Aug 10, 2021 13:25 UTC   364d            etcd-ca                 no
								// apiserver-kubelet-client   Aug 10, 2021 13:25 UTC   364d            ca                      no
								// controller-manager.conf    Aug 10, 2021 13:26 UTC   364d                                    no
								// etcd-healthcheck-client    Aug 10, 2021 13:25 UTC   364d            etcd-ca                 no
								// etcd-peer                  Aug 10, 2021 13:25 UTC   364d            etcd-ca                 no
								// etcd-server                Aug 10, 2021 13:25 UTC   364d            etcd-ca                 no
								// front-proxy-client         Aug 10, 2021 13:25 UTC   364d            front-proxy-ca          no
								// scheduler.conf             Aug 10, 2021 13:26 UTC   364d                                    no
								//
								// CERTIFICATE AUTHORITY   EXPIRES                  RESIDUAL TIME   EXTERNALLY MANAGED
								// ca                      Jul 30, 2021 12:18 UTC   353d            no			/etc/kubernetes/pki/ca.crt
								// etcd-ca                 Jul 30, 2021 12:18 UTC   353d            no			/etc/kubernetes/pki/etcd/ca.crt
								// front-proxy-ca          Jul 30, 2021 12:18 UTC   353d            no			/etc/kubernetes/pki/front-proxy-ca.crt

								line := strings.Split(data, "\n")

								for i := range line {
									// TODO: Check if UTC would be the right candidate to pattern match
									if strings.Contains(line[i], "UTC") {
										certLine := strings.Fields(line[i])[6]

										var durationStamp int
										if strings.ContainsAny(certLine, "dhms") {
											if !strings.Contains(certLine, "d") {
												durationStamp = 0
											} else {
												chk := regexp.MustCompile(`[d,h,m,s]`)

												durationStamp, err = strconv.Atoi(chk.Split(certLine, -1)[0])

												if err != nil {
													return nil, err
												}
											}
										}

										if durationStamp < 0 || durationStamp < int(d) {
											s := fmt.Sprintf("%s\t%s\t%s", strings.Fields(line[i])[0], hostName, strings.Fields(line[i])[1:6])
											fmt.Fprintln(w, s)

											flag = true

										}
									}
								}

								if flag {
									w.Flush()
								}
							}
						}
					}
				}
			}
		}
	}

	return nil, nil

}

// checkNotAfter fetches the notAfter data from the PEM block
func checkNotAfter(certData []byte, d int) (bool, string, error) {

	block, _ := pem.Decode(certData)

	if block == nil {

		return false, "", fmt.Errorf("failed to parse certificate PEM")

	}

	cert, err := x509.ParseCertificate(block.Bytes)

	if err != nil {

		return false, "", fmt.Errorf("failed to parse certificate: " + err.Error())

	}

	//      fmt.Println(cert.NotAfter.Date())
	//      fmt.Println(time.Now().Date())

	return checkIfExpired(cert.NotAfter, d), fmt.Sprintf("%v", cert.NotAfter), nil

}

// checkIfExpired checks if the certificate NotAfter is within the duration (input)
func checkIfExpired(notAfter time.Time, duration int) bool {

	diffTime := notAfter.Sub(time.Now())

	if int(diffTime.Hours()/24) < int(duration) {
		return true
	} else if int(diffTime.Hours()/24) < 0 {
		return true
	}

	return false
}

func kHeader() {
	doOnce.Do(func() {

		fmt.Println("")
		fmt.Println("##########################################")
		fmt.Println("  KUBECONFIG expiry of Workload Clusters  ")
		fmt.Println("##########################################")

	})
}
