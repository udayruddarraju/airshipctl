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

package v1alpha1

import (
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/runtime/schema"
	yaml2 "sigs.k8s.io/yaml"

	plugtypes "opendev.org/airship/airshipctl/pkg/document/plugin/types"
	"opendev.org/airship/airshipctl/pkg/environment"
	"opendev.org/airship/airshipctl/pkg/k8s/client"
	"opendev.org/airship/airshipctl/pkg/secret/sops"
)

// GetGVK returns group, version, kind object used to register version
// of the plugin
func GetGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "airshipit.org",
		Version: "v1alpha1",
		Kind:    "Sops",
	}
}

// New creates new instance of the plugin
func New(settings *environment.AirshipCTLSettings, cfg []byte) (plugtypes.Plugin, error) {
	s := &Sops{}
	if err := yaml2.Unmarshal(cfg, s); err != nil {
		return nil, err
	}

	s.settings = settings
	return s, nil
}

// Run sops plugin
func (s *Sops) Run(_ io.Reader, out io.Writer) error {
	kclient, err := client.DefaultClient(s.settings)
	if err != nil {
		return err
	}

	airconfig := s.settings.Config
	context, err := airconfig.GetCurrentContext()
	if err != nil {
		return err
	}
	sopsClient := sops.NewClient(kclient, context.ClusterName(), context.KubeContext().Namespace)
	if err = sopsClient.InitializeKeys(); err != nil {
		return err
	}

	output, err := sopsClient.Decrypt(s.File, "")
	if err != nil {
		return err
	}

	if _, err = fmt.Fprint(out, string(output)); err != nil {
		return err
	}
	return nil
}
