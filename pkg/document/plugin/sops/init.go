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

package sops

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	sopsv1alpha1 "opendev.org/airship/airshipctl/pkg/document/plugin/sops/v1alpha1"
	"opendev.org/airship/airshipctl/pkg/document/plugin/types"
)

// RegisterPlugin registers BareMetalHost generator plugin
func RegisterPlugin(registry map[schema.GroupVersionKind]types.Factory) {
	registry[sopsv1alpha1.GetGVK()] = sopsv1alpha1.New
}
