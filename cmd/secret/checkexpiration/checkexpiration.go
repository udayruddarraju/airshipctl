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

	// Checking the EXPIRY of TLS Secrets
	secretType := "kubernetes.io/tls"
	secrets, err := kclient.ClientSet().CoreV1().Secrets("").List(metav1.ListOptions{FieldSelector: fmt.Sprintf("type=%s", secretType)})

	for _, secret := range secrets.Items {

		fmt.Println("Checking ", secret.Name, "in ", secret.Namespace)

		//              fmt.Println(string(secret.Data["tls.crt"]))

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

			//                      fmt.Println("found ", kSecret.Name)

			for ownerref := range kSecret.OwnerReferences {
				//                              fmt.Println("kind " + kSecret.OwnerReferences[ownerref].Kind)

				if kSecret.OwnerReferences[ownerref].Kind == "KubeadmControlPlane" {
					fmt.Println("Checking ", kSecret.Name, " in ", kSecret.Namespace)
					kubecontent, err := clientcmd.Load(kSecret.Data["value"])
					if err != nil {
						return nil, err
					}
					//                                      fmt.Println(kubecontent.Clusters)

					// Iterate through each Cluster and identify expiry
					for clusterName, clusterData := range kubecontent.Clusters {
						//              fmt.Println(clusterName, string(clusterData.CertificateAuthorityData))
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
						//              fmt.Println(userName, string(userData.ClientCertificateData))

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

	// TODO Checking the expiry of Workload nodes certificates

	gvr := schema.GroupVersionResource{
		Group:    "hostconfig.airshipit.org",
		Version:  "v1alpha1",
		Resource: "hostconfigs",
	}

	hcList, err := kclient.DynamicClient().Resource(gvr).List(metav1.ListOptions{})

	if err != nil {
		return nil, err
	}

	for _, hc := range hcList.Items {
		//		fmt.Println("Produced by List call - ", hc.GetName())

		//              fld, exists, err := unstructured.NestedString(hc.Object, "status", "hostConfigStatus", "dtc-dtc-control-plane-vpsd5", "Checking certs", "stdout")

		fld, exists, err := unstructured.NestedMap(hc.Object, "status", "hostConfigStatus")

		if err != nil {
			return nil, err

		}
		if !exists {
			fmt.Println("Doesnot exist")
		}

		//              fmt.Println(fld)

		fmt.Println("Checking the Expiry on the Workload Nodes")
		//   fmt.Println(fld["dtc-dtc-control-plane-vpsd5"])

		w := tabwriter.NewWriter(os.Stdout, 2, 6, 3, ' ', 0)
		fmt.Fprintln(w, "===========\t========")
		fmt.Fprintln(w, "CERTIFICATE\tHOSTNAME")
		fmt.Fprintln(w, "===========\t========")

		flag := false

		for hostName, value := range fld {

			//			fmt.Println(hostName)
			//			fmt.Println(fld[hostName]) // This line works

			//			fmt.Println(value)
			for k, v := range value.(map[string]interface{}) {
				//				fmt.Println("Key", k)
				//				fmt.Println("value", v)
				//				fmt.Println("========")

				if k == "Execute kubeadm command cert expirataion" {
					if v.(map[string]interface{})["status"] == "Successful" {
						//					fmt.Println(v.(map[string]interface{})["results"])
						for _, list := range v.(map[string]interface{})["results"].([]interface{}) {
							//							fmt.Println("This is V1")

							//								fmt.Println("This is i1")
							//								fmt.Println(i)
							//								fmt.Println("This is kk")
							//								fmt.Println(kk)
							if list.(map[string]interface{})["status"] == "Successful" {
								//								fmt.Println(list.(map[string]interface{})["stdout"])
								//								fmt.Println(flag)

								data := fmt.Sprintf("%v", list.(map[string]interface{})["stdout"])

								line := strings.Split(data, "\n")

								for i := range line {
									//									fmt.Println(line[i])

									//									fmt.Println("====end====")

									if strings.Contains(line[i], "UTC") {
										certLine := strings.Fields(line[i])[6]

										//										fmt.Println(certLine)

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

										//										fmt.Println(durationStamp)

										if durationStamp < 0 || durationStamp < int(d) {
											//											fmt.Println("Expired", strings.Fields(line[i])[0], hostName)

											s := fmt.Sprintf("%s\t%s", strings.Fields(line[i])[0], hostName)
											fmt.Fprintln(w, s)

											flag = true

										}
									}
								}

								if flag {
									w.Flush()
								}

								//							fmt.Println("Printing cert")
								//								fmt.Println(list.(map[string]interface{})["path"], " -", list.(map[string]interface{})["expiryDate"])

								//									fmt.Println(list.(map[string]interface{})["item"])

								// yourDate, err := time.Parse("2006-01-02", fmt.Sprintf("%v", list.(map[string]interface{})["stdout"]))
								// if err != nil {
								// 	return nil, err
								//
								// }
								//								fmt.Println(yourDate)

								// output := checkIfExpired(yourDate, d)
								//
								// if output {
								// 	s := fmt.Sprintf("%s\t%s", list.(map[string]interface{})["path"], hostName)
								// 	fmt.Fprintln(w, s)
								// 	flag = true
								// 	//									fmt.Println("Expired")
								// }
								// if flag {
								// 	w.Flush()
								// }
							}
						}
					}

				}
			}
		}
	}

	// Below block works
	// _, err = json.Marshal(fld)
	//
	// if err != nil {
	// 	return nil, err
	//
	// }

	//              fmt.Println(string(statusData))

	// for hosts := range statusData {
	//      fmt.Println("Hosts")
	//      //              fmt.Println(statusData[hosts])
	// }

	//              fld = "2021-07-30"

	// if strings.HasPrefix(fld, "notAfter=") {
	//      split := strings.Split(fld, "=")
	//      fmt.Println(split[0])
	//      fmt.Println(split[1])
	//
	//      yourDate, err := time.Parse("Jul 30 03:20:40 2021 GMT", split[1])
	//      if err != nil {
	//              return nil, err
	//      }
	//      fmt.Println(yourDate)

	//      }
	//      unstructured.UnstructuredList()

	//      unstructuredObj := hc.

	//              unstructuredSpec := unstructuredObj.UnstructuredContent()

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

	//      fmt.Println(cert.NotAfter.Date())
	//      fmt.Println(time.Now().Date())

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

func expiryTemplate() {
	const expiry = `

	`
}

