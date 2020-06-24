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

package cluster_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"opendev.org/airship/airshipctl/pkg/config"

	"opendev.org/airship/airshipctl/cmd/cluster"
	"opendev.org/airship/airshipctl/pkg/environment"
	"opendev.org/airship/airshipctl/pkg/k8s/client"
	"opendev.org/airship/airshipctl/pkg/k8s/client/fake"
	"opendev.org/airship/airshipctl/testutil"
)

const (
	machineTemplateName     = "test-machine-template"
	kubeadmControlPlaneName = "test-control-plane"
)

func TestRenewCertsCmd(t *testing.T) {
	tests := []struct {
		cmdTest   *testutil.CmdTest
		resources []runtime.Object
	}{
		{
			cmdTest: &testutil.CmdTest{
				Name:    "check-renew-certs",
				CmdLine: "",
			},
			resources: []runtime.Object{
				makeKubeadmControlPlaneResource(kubeadmControlPlaneName),
				makeMachineTemplateResource(machineTemplateName),
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		testClientFactory := func(_ *environment.AirshipCTLSettings) (client.Interface, error) {
			return fake.NewClient(
				fake.WithDynamicObjects(tt.resources...),
			), nil
		}
		tt.cmdTest.Cmd = cluster.NewRenewCertsCommand(renewCertsTestSettings(), testClientFactory)
		testutil.RunTest(t, tt.cmdTest)
	}
}

func renewCertsTestSettings() *environment.AirshipCTLSettings {
	return &environment.AirshipCTLSettings{
		Config: &config.Config{
			Clusters:  map[string]*config.ClusterPurpose{"testCluster": nil},
			AuthInfos: map[string]*config.AuthInfo{"testAuthInfo": nil},
			Contexts: map[string]*config.Context{
				"testContext": {Manifest: "testManifest"},
			},
			Manifests: map[string]*config.Manifest{
				"testManifest": {TargetPath: fixturesPath},
			},
			CurrentContext: "testContext",
		},
	}
}

func makeMachineTemplateResource(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
			"kind":       "DockerMachineTemplate",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": "default",
			},
		},
	}
}

func makeKubeadmControlPlaneResource(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "controlplane.cluster.x-k8s.io/v1alpha3",
			"kind":       "KubeadmControlPlane",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"infrastructureTemplate": map[string]interface{}{
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
					"kind":       "DockerMachineTemplate",
					"name":       machineTemplateName,
					"namespace":  "default",
				},
			},
		},
	}
}
