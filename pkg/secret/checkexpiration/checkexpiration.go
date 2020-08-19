package checkexpiration

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientcmd "k8s.io/client-go/tools/clientcmd"
	"opendev.org/airship/airshipctl/pkg/environment"
	"opendev.org/airship/airshipctl/pkg/k8s/client"
)

type checkExpiry struct {
	TlsSecret []secretInfo  `json:"TlsSecret,omitempty" yaml:"TlsSecret,omitempty"`
	Kubeconf  []kSecretInfo `json:"Kubeconf,omitempty" yaml:"Kubeconf,omitempty"`
	NodeCert  []nCertInfo   `json:"NodeCert,omitempty" yaml:"NodeCert,omitempty"`
}

type secretInfo struct {
	SecretName      string            `json:"SecretName,omitempty" yaml:"SecretName,omitempty"`
	SecretNamespace string            `json:"SecretNamespace,omitempty" yaml:"SecretNamespace,omitempty"`
	Data            []certificateInfo `json:"Data,omitempty" yaml:"Data,omitempty"`
}

type certificateInfo struct {
	CertificateName string `json:"CertificateName,omitempty" yaml:"CertificateName,omitempty"`
	ExpiryDate      string `json:"ExpiryDate,omitempty" yaml:"ExpiryDate,omitempty"`
}

type kSecretInfo struct {
	SecretName      string        `json:"SecretName,omitempty" yaml:"SecretName,omitempty"`
	SecretNamespace string        `json:"SecretNamespace,omitempty" yaml:"SecretNamespace,omitempty"`
	Cluster         []clusterInfo `json:"Cluster,omitempty" yaml:"Cluster,omitempty"`
	User            []userInfo    `json:"User,omitempty" yaml:"User,omitempty"`
}

type clusterInfo struct {
	Name            string `json:"Name,omitempty" yaml:"Name,omitempty"`
	CertificateName string `json:"CertificateName,omitempty" yaml:"CertificateName,omitempty"`
	ExpiryDate      string `json:"ExpiryDate,omitempty" yaml:"ExpiryDate,omitempty"`
}

type userInfo struct {
	Name            string `json:"Name,omitempty" yaml:"Name,omitempty"`
	CertificateName string `json:"CertificateName,omitempty" yaml:"CertificateName,omitempty"`
	ExpiryDate      string `json:"ExpiryDate,omitempty" yaml:"ExpiryDate,omitempty"`
}

type nCertInfo struct {
	NodeName      string            `json:"NodeName,omitempty" yaml:"NodeName,omitempty"`
	NodeNamespace string            `json:"NodeNamespace,omitempty" yaml:"NodeNamespace,omitempty"`
	Data          []certificateInfo `json:"Data,omitempty" yaml:"Data,omitempty"`
}

//CheckexpiryData checks the expiry data of 1. TLS Secrets 2. Workload Cluster kubeconfig secret 3. Workload node Certificates
func CheckexpiryData(rootSettings *environment.AirshipCTLSettings, factory client.Factory, duration string, contentType string) error {

	d, err := strconv.Atoi(duration)

	if err != nil {
		return err
	}

	kclient, err := factory(rootSettings)
	if err != nil {
		return err
	}

	tlsData, err := checkTLS(kclient, d)

	if err != nil {
		return err
	}

	kSecretData, err := checkKubeconf(kclient, d)
	if err != nil {
		return err
	}

	nodeData, err := checkWorkloadNodes(kclient, d)
	if err != nil {
		return err
	}

	//      airconfig := rootSettings.Config
	//      _, err := airconfig.GetCurrentContext()
	//      if err != nil {
	//              return nil, err
	//      }

	checkexpiry := checkExpiry{
		TlsSecret: tlsData,
		Kubeconf:  kSecretData,
		NodeCert:  nodeData,
	}

	// Below just parses to YAML/JSON
	if contentType == "yaml" {

		dataYaml := parseYaml(checkexpiry)

		if dataYaml != "" && strings.TrimSpace(dataYaml) != "{}" {
			//	fmt.Println(dataYaml)
			fmt.Fprint(os.Stdout, dataYaml)
		}
	} else if contentType == "json" {
		dataJson := parseJson(checkexpiry)

		if dataJson != "" && strings.TrimSpace(dataJson) != "{}" {
			//	fmt.Println(dataJson)
			fmt.Fprint(os.Stdout, dataJson)
		}
	}

	return nil

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

func parseYaml(checkexpiry checkExpiry) string {

	buffer, err := yaml.Marshal(checkexpiry)

	if err != nil {
		fmt.Errorf("Unable to parse to YAML %s", err.Error())
		return ""
	}

	return string(buffer)

}

func parseJson(checkexpiry checkExpiry) string {

	//	buffer, err := json.Marshal(checkexpiry)
	buffer, err := json.MarshalIndent(checkexpiry, "", "    ")

	if err != nil {
		fmt.Errorf("Unable to parse to JSON  %s", err.Error())
		return ""
	}

	return string(buffer)

}

// checkTLS - to check the expiry of TLS Secrets
func checkTLS(kclient client.Interface, d int) ([]secretInfo, error) {

	secretType := "kubernetes.io/tls"
	secrets, err := kclient.ClientSet().CoreV1().Secrets("").List(metav1.ListOptions{FieldSelector: fmt.Sprintf("type=%s", secretType)})

	if err != nil {
		return nil, err
	}

	tlsData := make([]secretInfo, 0)
	for _, secret := range secrets.Items {

		secretData := make([]certificateInfo, 0)

		//		fmt.Println("Checking ", secret.Name, "in ", secret.Namespace)
		if secret.Data["tls.crt"] != nil {
			expiry, notAfter, err := checkNotAfter([]byte(secret.Data["tls.crt"]), d)
			if err != nil {
				return nil, err
			}

			if expiry {

				certificateinfo := certificateInfo{
					CertificateName: "tls.crt",
					ExpiryDate:      notAfter,
				}

				secretData = append(secretData, certificateinfo)

			}
		}

		if secret.Data["ca.crt"] != nil {

			expiry, notAfter, err := checkNotAfter([]byte(secret.Data["ca.crt"]), d)
			if err != nil {
				return nil, err
			}

			if expiry {

				certificateinfo := certificateInfo{
					CertificateName: "ca.crt",
					ExpiryDate:      notAfter,
				}

				secretData = append(secretData, certificateinfo)

			}

		}

		if len(secretData) > 0 {

			secretinfo := secretInfo{
				SecretName:      secret.Name,
				SecretNamespace: secret.Namespace,
				Data:            secretData,
			}

			tlsData = append(tlsData, secretinfo)

		}

	}

	return tlsData, nil

}

// checkKubeconf - fetches all the -kubeconfig secrets and identifies the expiry
func checkKubeconf(kclient client.Interface, d int) ([]kSecretInfo, error) {

	kSecrets, err := kclient.ClientSet().CoreV1().Secrets("").List(metav1.ListOptions{})

	if err != nil {
		return nil, err
	}

	kSecretData := make([]kSecretInfo, 0)
	for _, kSecret := range kSecrets.Items {

		if strings.HasSuffix(kSecret.Name, "-kubeconfig") {

			for ownerref := range kSecret.OwnerReferences {
				if kSecret.OwnerReferences[ownerref].Kind == "KubeadmControlPlane" {
					//					fmt.Println("Checking ", kSecret.Name, " in ", kSecret.Namespace)
					kubecontent, err := clientcmd.Load(kSecret.Data["value"])
					if err != nil {
						return nil, err
					}

					clusterDat := make([]clusterInfo, 0)
					// Iterate through each Cluster and identify expiry
					for clusterName, clusterData := range kubecontent.Clusters {
						expiry, notAfter, err := checkNotAfter(clusterData.CertificateAuthorityData, d)

						if err != nil {
							return nil, err
						}

						if expiry {

							//							fmt.Fprint(os.Stdout, "CertificateAuthorityData for "+clusterName+" cluster in "+kSecret.Name+" expires on ["+notAfter+"]\n")

							clusterinfo := clusterInfo{
								Name:            clusterName,
								CertificateName: "CertificateAuthorityData",
								ExpiryDate:      notAfter,
							}

							clusterDat = append(clusterDat, clusterinfo)

						}
					}

					userDat := make([]userInfo, 0)
					// Iterate through each user Certificate and identify expiry
					for userName, userData := range kubecontent.AuthInfos {

						expiry, notAfter, err := checkNotAfter(userData.ClientCertificateData, d)

						if err != nil {
							return nil, err
						}

						if expiry {

							//							fmt.Fprint(os.Stdout, "ClientCertificateData for "+userName+" user in  "+kSecret.Name+" expires on ["+notAfter+"]\n")
							userinfo := userInfo{
								Name:            userName,
								CertificateName: "ClientCertificateData",
								ExpiryDate:      notAfter,
							}

							userDat = append(userDat, userinfo)
						}
					}

					if len(clusterDat) > 0 || len(userDat) > 0 {
						kSecretinfo := kSecretInfo{
							SecretName:      kSecret.Name,
							SecretNamespace: kSecret.Namespace,
							Cluster:         clusterDat,
							User:            userDat,
						}
						kSecretData = append(kSecretData, kSecretinfo)
					}
				}
			}

		}

	}

	return kSecretData, nil

}

// checkWorkloadNodes - checks the expiry of certificates in the Workload control plane nodes
func checkWorkloadNodes(kclient client.Interface, d int) ([]nCertInfo, error) {

	// Understanding is the node will be updated with an annotation with the expiry content (Activity of HostConfig Operator -
	// 'check-expiry' CR Object) every day (Cron like activity is performed by reconcile tag in the Operator)
	// Below code is implemented to just read the annotation, parse it, identify expirable content and report back

	nodes, err := kclient.ClientSet().CoreV1().Nodes().List(metav1.ListOptions{})

	if err != nil {
		return nil, err
	}

	nodeData := make([]nCertInfo, 0)
	for _, node := range nodes.Items {

		//		fmt.Println(node.ObjectMeta.Annotations)

		for key, value := range node.ObjectMeta.Annotations {

			if key == "cert-expiration" {

				replaced := strings.ReplaceAll(value, "{", "")
				split := strings.Split(replaced, "},")

				certificateData := make([]certificateInfo, 0)
				for _, v := range split {
					cName := strings.TrimSpace(strings.Split(v, ":")[0])
					cData := strings.TrimSpace(strings.Split(v, ":")[1]) + ":" + strings.TrimSpace(strings.ReplaceAll(strings.Split(v, ":")[2], "}", ""))

					fdate, err := time.Parse("Jan 02, 2006 15:04 MST", cData)

					if err != nil {
						return nil, err
					}

					if checkIfExpired(fdate, d) {

						certificateinfo := certificateInfo{
							CertificateName: cName,
							ExpiryDate:      fmt.Sprintf("%v", fdate),
						}

						certificateData = append(certificateData, certificateinfo)

					}

				}

				if len(certificateData) > 0 {
					ncertinfo := nCertInfo{
						NodeName:      node.ObjectMeta.Name,
						NodeNamespace: node.ObjectMeta.Namespace,
						Data:          certificateData,
					}
					nodeData = append(nodeData, ncertinfo)
				}

			}

		}
	}

	return nodeData, nil
}
