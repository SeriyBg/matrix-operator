/*
Copyright 2023.

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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AgentName string

const (
	Smith    AgentName = "Smith"
	Brown    AgentName = "Brown"
	Jones    AgentName = "Jones"
	Jackson  AgentName = "Jackson"
	Johnson  AgentName = "Johnson"
	Thompson AgentName = "Thompson"
)

// AgentSpec defines the desired state of Agent
type AgentSpec struct {
	Agents []SingleAgent `json:"agents"`
}

type SingleAgent struct {
	Name AgentName `json:"name"`

	// +kubebuilder:default:=100
	// +optional
	Health *int32 `json:"health"`
}

// AgentStatus defines the observed state of Agent
type AgentStatus struct {
	AgentsAlive string             `json:"agentsAlive"`
	Agents      []SingeAgentStatus `json:"agents"`
}

type SingeAgentStatus struct {
	Name   AgentName `json:"name"`
	Health int32     `json:"health"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Alive",type="string",JSONPath=".status.agentsAlive",description="Current number of alive agents out of all"
//+kubebuilder:storageversion

// Agent is the Schema for the agents API
type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSpec   `json:"spec,omitempty"`
	Status AgentStatus `json:"status,omitempty"`
}

func (in *Agent) ProgramName() string {
	return "agent"
}

//+kubebuilder:object:root=true

// AgentList contains a list of Agent
type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Agent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Agent{}, &AgentList{})
}
