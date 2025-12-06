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

// ExchangeParameters are the configurable fields of a Exchange.
// +kubebuilder:validation:Required
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.name) || self.name == oldSelf.name",message="Name is immutable once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.vhost) || self.vhost == oldSelf.vhost",message="Vhost is immutable once"
type ExchangeParameters struct {
	Name             *string           `json:"name,omitempty"`
	Vhost            string            `json:"vhost"`
	ExchangeSettings *ExchangeSettings `json:"exchangeSettings,omitempty"`
}

// ExchangeSettings
// +kubebuilder:validation:Required
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.type) || self.type == oldSelf.type",message="Type is immutable once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.durable) || self.durable == oldSelf.durable",message="Durable is immutable once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.autoDelete) || self.autoDelete == oldSelf.autoDelete",message="AutoDelete is immutable once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.arguments) || self.arguments == oldSelf.arguments",message="Arguments are immutable once set"
type ExchangeSettings struct {
	Type       *string           `json:"type,omitempty"`
	Durable    *bool             `json:"durable,omitempty"`
	AutoDelete *bool             `json:"autoDelete,omitempty"`
	Arguments  map[string]string `json:"arguments,omitempty"`
}

// ExchangeObservation are the observable fields of a Exchange.
type ExchangeObservation struct {
	Name       string            `json:"name,omitempty"`
	Vhost      string            `json:"vhost,omitempty"`
	Type       string            `json:"type,omitempty"`
	Durable    bool              `json:"durable,omitempty"`
	AutoDelete bool              `json:"autoDelete,omitempty"`
	Arguments  map[string]string `json:"arguments,omitempty"`
}

// A ExchangeSpec defines the desired state of a Exchange.
type ExchangeSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       ExchangeParameters `json:"forProvider"`
}

// A ExchangeStatus represents the observed state of a Exchange.
type ExchangeStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          ExchangeObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// Exchange - manage exchanges
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,rabbitmq}
type Exchange struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExchangeSpec   `json:"spec"`
	Status ExchangeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ExchangeList contains a list of Exchange
type ExchangeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Exchange `json:"items"`
}

// Exchange type metadata.
var (
	ExchangeKind             = reflect.TypeOf(Exchange{}).Name()
	ExchangeGroupKind        = schema.GroupKind{Group: Group, Kind: ExchangeKind}.String()
	ExchangeKindAPIVersion   = ExchangeKind + "." + SchemeGroupVersion.String()
	ExchangeGroupVersionKind = SchemeGroupVersion.WithKind(ExchangeKind)
)

func init() {
	SchemeBuilder.Register(&Exchange{}, &ExchangeList{})
}
