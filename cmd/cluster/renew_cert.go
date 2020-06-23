/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cluster

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"os"
	"sigs.k8s.io/cluster-api/api/v1alpha3"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"opendev.org/airship/airshipctl/pkg/environment"
	"opendev.org/airship/airshipctl/pkg/k8s/client"
)

const (
	renewCertLong = `
Renews control plane certificates if the expiration threshold is met followed by restarting control plane components.
`

	renewCertExample = `
Renew certificates in place whe their expiration is in the next 24 hours.
  airshipctl cluster renew-certs --expiration-threshold="24h"

Renew certificates by triggering a rolling update using the clusterapi management control plane.
  airshipctl cluster renew-certs --expiration-threshold="24h" --renew-in-place=False
`

	maxPodStatusCheckRetries  = 30
	maxNodeStatusCheckRetries = 30
)

// NewMoveCommand creates a command to move capi and bmo resources to the target cluster
func NewRenewCertsCommand(rootSettings *environment.AirshipCTLSettings, factory client.Factory) *cobra.Command {
	var expirationThresholdInput string
	var renewInPlace bool
	moveCmd := &cobra.Command{
		Use:     "renew-certs",
		Short:   "Renew control plane certificates close to expiration and restart control plane components",
		Long:    renewCertLong[1:],
		Example: renewCertExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := renewCerts(rootSettings, factory, expirationThresholdInput, renewInPlace)
			if err != nil {
				return fmt.Errorf("failed renewing certificates: %s", err.Error())
			}
			fmt.Fprint(os.Stdout, "successfully renewed control plane certificates on all nodes\n")
			return nil
		},
	}

	moveCmd.Flags().StringVar(&expirationThresholdInput, "expiration-threshold", "24h",
		"Expiration threshold which when met will trigger certificate renewal of certificates used by control plane components in a duration format. Defaults to 24h. Valid time units are \"ns\", \"us\" (or \"µs\"), \"ms\", \"s\", \"m\", \"h\".")
	moveCmd.Flags().BoolVar(&renewInPlace, "renew-in-place", true,
		"Renew certificates without triggering a rolling update which leads to replacing of old control plane instances from the provider. Defaults to true.")
	return moveCmd
}

func renewCerts(rootSettings *environment.AirshipCTLSettings, factory client.Factory, expirationThreshold string, renewInPlace bool) error {
	if renewInPlace {
		return renewCertsInPlace(rootSettings, factory, expirationThreshold)
	}
	return renewCertsRollingUpdate(rootSettings, factory, expirationThreshold)
}

func renewCertsRollingUpdate(rootSettings *environment.AirshipCTLSettings, factory client.Factory, expirationThreshold string) error {
	f, err := factory(rootSettings)
	if err != nil {
		return err
	}
	gvr := schema.GroupVersionResource{
		Group: v1alpha3.GroupVersion.Group,
		Version: v1alpha3.GroupVersion.Version,
		Resource: "machines",
	}
	machineList, err := f.DynamicClient().Resource(gvr).List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, machine := range machineList.Items {
		fmt.Println(machine.GetName())
	}
	// todo: create a new client to the cluster api management control plane

	// todo: find the provider, and retrieve the appropriate machine template for the control plane

	// todo: change the name of the machine template and create it in the management control plane

	// todo: change the reference to the machinetemplate from the KubeadmControlPlaneConfiguration to point
	// to the new machine template and update the object in the management api control plane.

	// todo: print directions to check the status of the operation

	// todo: clarify: is it ok to use clusterctl to check for the status of the rolling update

	// todo: clarify: is it ok to do a fire and forget from airshipctl ot should we track the rolling update from
	// airshipctl. If we want to track the progress, we might also want to add a new flag to just do fire and forget
	// so users with clusterctl can still be able to just fire rolling update from airshipctl and later use clusterctl
	// to verify the status of the rolling update. This might help us to prevent a lot of duplicate logic in airshipctl
	// that

	return nil
}

func renewCertsInPlace(rootSettings *environment.AirshipCTLSettings, factory client.Factory, expirationThreshold string) error {
	fmt.Fprintf(os.Stdout, fmt.Sprintf("Starting certificate renewal for all masters nodes, one at a time, with expiration threshold set to %s\n", expirationThreshold))
	kclient, err := factory(rootSettings)
	if err != nil {
		return fmt.Errorf("unable to get kube client from factory: %s\n", err.Error())
	}

	// get all master nodes
	masterNodesList, err := getMasterNodes(kclient)
	if err != nil {
		return fmt.Errorf("unable to list masters from the api: %s\n", err.Error())
	}

	fmt.Fprintf(os.Stdout, fmt.Sprintf("Got %d master nodes\n", len(masterNodesList.Items)))

	for _, masterNode := range masterNodesList.Items {
		// renew cert on each master node
		fmt.Fprintf(os.Stdout, fmt.Sprintf("Starting certificate renewal on node %s\n", masterNode.Name))
		pod := generateRenewPod(&masterNode, expirationThreshold)

		fmt.Fprintf(os.Stdout, fmt.Sprintf("Creating pod %s on node %s to renew certificates if needed\n", pod.GenerateName, masterNode.Name))
		createdPod, err := kclient.ClientSet().CoreV1().Pods("kube-system").Create(pod)
		if err != nil {
			return fmt.Errorf("creating pod %s on node %s failed: %s\n", pod.GenerateName, masterNode.Name, err.Error())
		}

		fmt.Fprintf(os.Stdout, fmt.Sprintf("Created pod %s on node %s\n", createdPod.Name, masterNode.Name))

		// wait for 10 seconds before checking for the health of the pod
		succeeded := false
		for i := 0; i < maxPodStatusCheckRetries; i++ {
			time.Sleep(10 * time.Second)
			// check of the pod to be completed with return code 0
			pod, err := kclient.ClientSet().CoreV1().Pods("kube-system").Get(createdPod.Name, metav1.GetOptions{})
			if err != nil {
				fmt.Fprintf(os.Stderr, fmt.Sprintf("check for pod status in apiserver failed: %s\n", err.Error()))
				continue
			}

			if pod.Status.Phase == corev1.PodSucceeded {
				succeeded = true
				break
			}
		}
		if !succeeded {
			return fmt.Errorf("pod %s did not succeed which indicates that the cert renewal did not work, check the pod logs for more details\n", pod.Name)
		}

		fmt.Fprintf(os.Stderr, fmt.Sprintf("pod %s succeeded, checking for the health of the node now\n", pod.Name))
		succeeded = false
		for i := 0; i < maxNodeStatusCheckRetries; i++ {
			time.Sleep(10 * time.Second)
			isHealthy, err := isNodeHealthy(masterNode.Name, kclient)
			if err != nil {
				fmt.Fprintf(os.Stderr, fmt.Sprintf("check for health of node %s failed :%s\n", masterNode.Name, err.Error()))
				continue
			}
			if isHealthy {
				succeeded = true
				break
			}
			fmt.Fprintf(os.Stdout, fmt.Sprintf("check for health of node %s failed, will retry in 10 seconds\n", masterNode.Name))
		}

		if !succeeded {
			return fmt.Errorf("node %s is not healthy after certificate renewal. Look into the kube apiserver and kubelets logs to find more details\n", masterNode.Name)
		}
		fmt.Fprintf(os.Stdout, "certificate renewal successfully completed on node %s, moving on to the next master node\n", masterNode.Name)
	}
	return nil
}

func generateRenewPod(node *corev1.Node, expirationThreshold string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("cert-renew-%s", node.Name),
			Labels: map[string]string{
				"app": "kadm-cert-rotate",
			},
			Annotations: map[string]string{},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "corev1",
		},
		Spec: corev1.PodSpec{
			HostPID: true,
			HostNetwork: true,
			Containers: []corev1.Container{
				{
					Image: "kadm-cert-rotate:v4",
					Name:  "kubeadm-cert-rotater",
					Command: []string{
						"/bin/sh",
						"-c",
						fmt.Sprintf("kadm-cert-rotate --renewal-threshold=\"%s\"\n", expirationThreshold),
					},
					SecurityContext: &corev1.SecurityContext{
						Capabilities: &corev1.Capabilities{
							Add: []corev1.Capability{
								"SYS_PTRACE",
							},
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "pki",
							MountPath: "/etc/kubernetes",
						},
						{
							Name:      "dockersock",
							MountPath: "/var/run/docker.sock",
						},
					},
				},
			},
			NodeName:      node.Name,
			RestartPolicy: corev1.RestartPolicyNever,
			Volumes: []corev1.Volume{
				{
					Name: "pki",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/etc/kubernetes",
						},
					},
				}, {
					Name: "dockersock",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/var/run/docker.sock",
						},
					},
				},
			},
		},
	}
}

func getMasterNodes(kclient client.Interface) (*corev1.NodeList, error) {
	// get all master nodes
	masterLabel := map[string]string{
		"node-role.kubernetes.io/master": "",
	}
	labelSelector := labels.Set(masterLabel).AsSelector()
	lsOption := metav1.ListOptions{
		LabelSelector: labelSelector.String(),
	}
	return kclient.ClientSet().CoreV1().Nodes().List(lsOption)
}

func isNodeHealthy(nodeName string, kclient client.Interface) (bool, error) {
	node, err := kclient.ClientSet().CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	// check for kubelet status
	isHealthy := false
	for _, condition := range node.Status.Conditions {
		if condition.Reason == "KubeletReady" {
			if condition.Status == corev1.ConditionTrue {
				isHealthy = true
			}
		}
	}
	return isHealthy, nil
}
