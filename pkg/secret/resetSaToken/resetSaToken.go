package resetSaToken

import (
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"opendev.org/airship/airshipctl/pkg/environment"
	"opendev.org/airship/airshipctl/pkg/k8s/client"
)

var secretType = "kubernetes.io/service-account-token"

// rotateToken - rotates the token 1. Deletes the secret and 2. Deletes its pod
func RotateToken(rootSettings *environment.AirshipCTLSettings, factory client.Factory, ns string, secretName string) error {
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
