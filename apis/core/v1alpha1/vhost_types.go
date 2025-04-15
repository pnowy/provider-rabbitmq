/*
Copyright 2022 The Crossplane Authors.

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

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// VhostParameters are the configurable fields of a Vhost
// +kubebuilder:validation:Required
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.hostName) || self.hostName == oldSelf.hostName",message="HostName is immutable once set"
type VhostParameters struct {
	HostName      *string        `json:"hostName,omitempty"`
	VhostSettings *VhostSettings `json:"vhostSettings,omitempty"`
}

type VhostSettings struct {
	// Virtual host description
	Description *string `json:"description,omitempty"`
	// Virtual host tags
	Tags VhostTags `json:"tags,omitempty"`
	// Type of queue to create in virtual host when unspecified in queue level
	DefaultQueueType *string `json:"defaultQueueType,omitempty"`
	// True if tracing should be enabled
	Tracing *bool `json:"tracing,omitempty"`
}

type VhostTags []string

// VhostObservation are the observable fields of a Vhost.
type VhostObservation struct {
	Name                   string  `json:"name"`
	Description            *string `json:"description,omitempty"`
	DefaultQueueType       *string `json:"defaultQueueType,omitempty"`
	Messages               int     `json:"messages"`
	MessagesReady          int     `json:"messagesReady"`
	MessagesUnacknowledged int     `json:"messagesUnacknowledged"`
}

// A VhostSpec defines the desired state of a Vhost.
type VhostSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       VhostParameters `json:"forProvider"`
}

// A VhostStatus represents the observed state of a Vhost.
type VhostStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          VhostObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A Vhost is an example API type.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,rabbitmq}
type Vhost struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VhostSpec   `json:"spec"`
	Status VhostStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VhostList contains a list of Vhost
type VhostList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Vhost `json:"items"`
}

// Vhost type metadata.
var (
	VhostKind             = reflect.TypeOf(Vhost{}).Name()
	VhostGroupKind        = schema.GroupKind{Group: Group, Kind: VhostKind}.String()
	VhostKindAPIVersion   = VhostKind + "." + SchemeGroupVersion.String()
	VhostGroupVersionKind = SchemeGroupVersion.WithKind(VhostKind)
)

func init() {
	SchemeBuilder.Register(&Vhost{}, &VhostList{})
}
