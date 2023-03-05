/*
Copyright 2022.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MerovingianSpec defines the desired state of Merovingian
type MerovingianSpec struct {
}

// MerovingianStatus defines the observed state of Merovingian
type MerovingianStatus struct {
	ExiledCount int32    `json:"exiledCount"`
	Exiled      []string `json:"exiled"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Exiled",type="integer",JSONPath=".status.exiledCount",description="Current count of exiled Matrix programs"

// Merovingian is the Schema for the merovingians API
type Merovingian struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MerovingianSpec   `json:"spec,omitempty"`
	Status MerovingianStatus `json:"status,omitempty"`
}

func (in *Merovingian) ProgramName() string {
	return "merovingian"
}

//+kubebuilder:object:root=true

// MerovingianList contains a list of Merovingian
type MerovingianList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Merovingian `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Merovingian{}, &MerovingianList{})
}
