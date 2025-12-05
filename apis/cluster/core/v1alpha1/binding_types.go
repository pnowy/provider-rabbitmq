/*
Copyright 2025 The Crossplane Authors.

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
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
)

// BindingParameters are the configurable fields of a Binding.
// +kubebuilder:validation:Required
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.destination) || self.destination == oldSelf.destination",message="Destination is immutable once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.vhost) || self.vhost == oldSelf.vhost",message="Vhost is immutable once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.destinationType) || self.destinationType == oldSelf.destinationType",message="Destination Type is immutable once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.source) || self.source == oldSelf.source",message="Source is immutable once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.routingKey) || self.routingKey == oldSelf.routingKey",message="Routing Key is immutable once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.arguments) || self.arguments == oldSelf.arguments",message="Arguments are immutable once set"
type BindingParameters struct {
	Source          string            `json:"source"`
	Vhost           string            `json:"vhost"`
	Destination     string            `json:"destination"`
	DestinationType string            `json:"destinationType"`
	RoutingKey      string            `json:"routingKey"`
	Arguments       map[string]string `json:"arguments,omitempty"`
}

// BindingObservation are the observable fields of a Binding.
type BindingObservation struct {
	Source          string            `json:"source"`
	Vhost           string            `json:"vhost"`
	Destination     string            `json:"destination"`
	DestinationType string            `json:"destinationType"`
	RoutingKey      string            `json:"routingKey"`
	PropertiesKey   string            `json:"propertiesKey"`
	Arguments       map[string]string `json:"arguments"`
}

// A BindingSpec defines the desired state of a Binding.
type BindingSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       BindingParameters `json:"forProvider"`
}

// A BindingStatus represents the observed state of a Binding.
type BindingStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          BindingObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// Binding - configure bindings between exchanges and queues
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,rabbitmq}
type Binding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BindingSpec   `json:"spec"`
	Status BindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BindingList contains a list of Binding
type BindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Binding `json:"items"`
}

// Binding type metadata.
var (
	BindingKind             = reflect.TypeOf(Binding{}).Name()
	BindingGroupKind        = schema.GroupKind{Group: Group, Kind: BindingKind}.String()
	BindingKindAPIVersion   = BindingKind + "." + SchemeGroupVersion.String()
	BindingGroupVersionKind = SchemeGroupVersion.WithKind(BindingKind)
)

func init() {
	SchemeBuilder.Register(&Binding{}, &BindingList{})
}
