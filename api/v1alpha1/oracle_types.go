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

// OracleSpec defines the desired state of Oracle
type OracleSpec struct {
	// +optional
	Config *OracleConfig `json:"config"`
}

type OracleConfig struct {
	Names       []string `json:"names,omitempty"`
	Predictions []string `json:"predictions,omitempty"`
}

// OracleStatus defines the observed state of Oracle
type OracleStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Oracle is the Schema for the oracles API
type Oracle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OracleSpec   `json:"spec,omitempty"`
	Status OracleStatus `json:"status,omitempty"`
}

func (in *Oracle) ProgramName() string {
	return "oracle"
}

//+kubebuilder:object:root=true

// OracleList contains a list of Oracle
type OracleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Oracle `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Oracle{}, &OracleList{})
}
