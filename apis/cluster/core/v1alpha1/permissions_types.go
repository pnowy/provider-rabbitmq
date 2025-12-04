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

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
)

// PermissionsParameters are the configurable fields of a Permissions.
// +kubebuilder:validation:Required
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.user) || self.user == oldSelf.user",message="User is immutable once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.vhost) || self.vhost == oldSelf.vhost",message="Vhost is immutable once set"
type PermissionsParameters struct {
	User               string              `json:"user"`
	Vhost              string              `json:"vhost"`
	PermissionSettings *PermissionSettings `json:"permissions"`
}

type PermissionSettings struct {
	Configure string `json:"configure"`
	Write     string `json:"write"`
	Read      string `json:"read"`
}

// PermissionsObservation are the observable fields of a Permissions.
type PermissionsObservation struct {
	User      string `json:"user"`
	Vhost     string `json:"vhost"`
	Configure string `json:"configure"`
	Write     string `json:"write"`
	Read      string `json:"read"`
}

// A PermissionsSpec defines the desired state of a Permissions.
type PermissionsSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       PermissionsParameters `json:"forProvider"`
}

// A PermissionsStatus represents the observed state of a Permissions.
type PermissionsStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          PermissionsObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// Permissions - manage user permissions within virtual hosts
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,rabbitmq}
type Permissions struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PermissionsSpec   `json:"spec"`
	Status PermissionsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PermissionsList contains a list of Permissions
type PermissionsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Permissions `json:"items"`
}

// Permissions type metadata.
var (
	PermissionsKind             = reflect.TypeOf(Permissions{}).Name()
	PermissionsGroupKind        = schema.GroupKind{Group: Group, Kind: PermissionsKind}.String()
	PermissionsKindAPIVersion   = PermissionsKind + "." + SchemeGroupVersion.String()
	PermissionsGroupVersionKind = SchemeGroupVersion.WithKind(PermissionsKind)
)

func init() {
	SchemeBuilder.Register(&Permissions{}, &PermissionsList{})
}
