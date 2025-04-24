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

// QueueParameters are the configurable fields of a Queue.
// +kubebuilder:validation:Required
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.name) || self.name == oldSelf.name",message="Name is immutable once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.vhost) || self.vhost == oldSelf.vhost",message="Vhost is immutable once"
type QueueParameters struct {
	Name          *string        `json:"name,omitempty"`
	Vhost         string         `json:"vhost"`
	QueueSettings *QueueSettings `json:"queueSettings,omitempty"`
}

// QueueSettings
// +kubebuilder:validation:Required
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.type) || self.type == oldSelf.type",message="Type is immutable once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.durable) || self.durable == oldSelf.durable",message="Durable is immutable once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.autoDelete) || self.autoDelete == oldSelf.autoDelete",message="AutoDelete is immutable once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.arguments) || self.arguments == oldSelf.arguments",message="Arguments are immutable once set"
type QueueSettings struct {
	Type       *string           `json:"type,omitempty"`
	Durable    *bool             `json:"durable,omitempty"`
	AutoDelete *bool             `json:"autoDelete,omitempty"`
	Arguments  map[string]string `json:"arguments,omitempty"`
}

// QueueObservation are the observable fields of a Queue.
type QueueObservation struct {
	Name       string            `json:"name,omitempty"`
	Vhost      string            `json:"vhost"`
	Type       string            `json:"type,omitempty"`
	Durable    bool              `json:"durable,omitempty"`
	AutoDelete bool              `json:"autoDelete,omitempty"`
	Arguments  map[string]string `json:"arguments,omitempty"`
}

// A QueueSpec defines the desired state of a Queue.
type QueueSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       QueueParameters `json:"forProvider"`
}

// A QueueStatus represents the observed state of a Queue.
type QueueStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          QueueObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// Queue - manage queues
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,rabbitmq}
type Queue struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   QueueSpec   `json:"spec"`
	Status QueueStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// QueueList contains a list of Queue
type QueueList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Queue `json:"items"`
}

// Queue type metadata.
var (
	QueueKind             = reflect.TypeOf(Queue{}).Name()
	QueueGroupKind        = schema.GroupKind{Group: Group, Kind: QueueKind}.String()
	QueueKindAPIVersion   = QueueKind + "." + SchemeGroupVersion.String()
	QueueGroupVersionKind = SchemeGroupVersion.WithKind(QueueKind)
)

func init() {
	SchemeBuilder.Register(&Queue{}, &QueueList{})
}
