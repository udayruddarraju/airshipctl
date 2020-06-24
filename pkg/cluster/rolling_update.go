package cluster

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/api/v1alpha3"
	v1alpha32 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"

	"opendev.org/airship/airshipctl/pkg/environment"
	"opendev.org/airship/airshipctl/pkg/k8s/client"
)

//  patchStringValue specifies a patch operation for a uint32.
type patchUInt32Value struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value"`
}

// RollingUpdateControlPlane triggers a rolling update on the target cluster's control plane
// using cluster-api. It does so by creating a new machine template and changing the reference
// of the infrastructure template from the kubeadmcontrolplane to point to the new machine template.
func RollingUpdateControlPlane(
	rootSettings *environment.AirshipCTLSettings,
	factory client.Factory, writer io.Writer,
) error {
	f, err := factory(rootSettings)
	if err != nil {
		return err
	}

	// get kubeadmcontrolplane for the target cluster
	kubeAdmControlPlane, err := getKubeadmControlPlane(f)
	if err != nil {
		return fmt.Errorf("getting kubeadmcontrolplane failed: %s", err.Error())
	}

	// get the machinetemplate referenced by the kubeadmcontrolplane
	machineTemplate, err := getMachineTemplateFromReference(f, kubeAdmControlPlane.Spec.InfrastructureTemplate)
	if err != nil {
		return fmt.Errorf("getting machine template failed: %s", err.Error())
	}

	// create a new machine template that is identical to the previous machine template
	// but a different name. We need accessor to retrieve the object here to keep the following
	// logic generic and agnostic of the provider.
	accessor, err := meta.Accessor(machineTemplate)
	if err != nil {
		return fmt.Errorf("reading machine template object from the apiserver failed: %s", err.Error())
	}
	accessor.SetGenerateName(fmt.Sprintf("%s-", accessor.GetName()))
	accessor.SetName("")
	accessor.SetResourceVersion("")
	newMachineTemplate, err := createMachineTemplate(
		f,
		machineTemplate,
		kubeAdmControlPlane.Spec.InfrastructureTemplate.Kind,
		kubeAdmControlPlane.Spec.InfrastructureTemplate.Namespace,
	)
	if err != nil {
		return fmt.Errorf("unable to create new machine template: %s", err.Error())
	}

	// update the reference from kubeadmcontrolplane to point to the new machine template
	err = updateKubeadmControlPlaneTemplate(f, kubeAdmControlPlane, newMachineTemplate)
	return nil
}

// get getKubeadmControlPlane returns the kubeadmcontrolplane of the target cluster
func getKubeadmControlPlane(f client.Interface) (*v1alpha32.KubeadmControlPlane, error) {
	gvrKcp := schema.GroupVersionResource{
		Group:    "controlplane." + v1alpha3.GroupVersion.Group,
		Version:  v1alpha3.GroupVersion.Version,
		Resource: "kubeadmcontrolplanes",
	}

	kadmControlPlanes, err := f.DynamicClient().Resource(gvrKcp).Namespace("").List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	if len(kadmControlPlanes.Items) > 0 {
		converter := runtime.DefaultUnstructuredConverter
		result := &v1alpha32.KubeadmControlPlane{}
		err := converter.FromUnstructured(kadmControlPlanes.Items[0].Object, result)
		return result, err
	}

	return nil, fmt.Errorf("no kubeadmcontrolplanes found in the management cluster")
}

// getMachineTemplateFromReference returns the machine template object referenced
// It returns a runtime.Object to ensure this logic is agnostic of the infrastructure provider
func getMachineTemplateFromReference(f client.Interface, reference corev1.ObjectReference) (runtime.Object, error) {
	gvrKcp := schema.GroupVersionResource{
		Group:    "infrastructure." + v1alpha3.GroupVersion.Group,
		Version:  v1alpha3.GroupVersion.Version,
		Resource: fmt.Sprintf("%s", strings.ToLower(reference.Kind)+"s"),
	}

	machineTemplate, err := f.DynamicClient().Resource(gvrKcp).
		Namespace(reference.Namespace).
		Get(reference.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return machineTemplate.DeepCopyObject(), nil
}

// createMachineTemplate creates a machinetemplate object for the infrastructure provider
func createMachineTemplate(
	f client.Interface, object runtime.Object,
	resourceKind string, namespace string) (runtime.Object, error) {
	gvr := schema.GroupVersionResource{
		Group:    "infrastructure." + v1alpha3.GroupVersion.Group,
		Version:  v1alpha3.GroupVersion.Version,
		Resource: fmt.Sprintf("%s", strings.ToLower(resourceKind)+"s"),
	}
	unstructuredObject, ok := object.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("expected type unstructured.Unstructured but received %s", reflect.TypeOf(object))
	}

	return f.DynamicClient().Resource(gvr).Namespace(namespace).Create(unstructuredObject, metav1.CreateOptions{})
}

// updateKubeadmControlPlaneTemplate updates the infrastructureTemplate reference on the KubeadmControlPlane
func updateKubeadmControlPlaneTemplate(f client.Interface, kcp *v1alpha32.KubeadmControlPlane,
	machineTemplate runtime.Object) error {
	gvr := schema.GroupVersionResource{
		Group:    "controlplane." + v1alpha3.GroupVersion.Group,
		Version:  v1alpha3.GroupVersion.Version,
		Resource: "kubeadmcontrolplanes",
	}

	machineTemplateAccessor, err := meta.Accessor(machineTemplate)
	if err != nil {
		return fmt.Errorf("unable to read the new machine template created: %s", err.Error())
	}

	patchPayload := []patchUInt32Value{{
		Op:    "replace",
		Path:  "/spec/infrastructureTemplate/name",
		Value: machineTemplateAccessor.GetName(),
	}}
	patchBytes, err := json.Marshal(patchPayload)
	if err != nil {
		return fmt.Errorf("error constructing json patch: %s", err.Error())
	}
	_, err = f.DynamicClient().Resource(gvr).
		Namespace(kcp.Namespace).
		Patch(kcp.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("unable to patch kubeadmcontrolplane with the new machine template reference: %s", err.Error())
	}

	return nil
}
