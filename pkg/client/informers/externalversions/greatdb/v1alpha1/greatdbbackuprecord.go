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
// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	greatdbv1alpha1 "greatdb-operator/pkg/apis/greatdb/v1alpha1"
	versioned "greatdb-operator/pkg/client/clientset/versioned"
	internalinterfaces "greatdb-operator/pkg/client/informers/externalversions/internalinterfaces"
	v1alpha1 "greatdb-operator/pkg/client/listers/greatdb/v1alpha1"
	time "time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// GreatDBBackupRecordInformer provides access to a shared informer and lister for
// GreatDBBackupRecords.
type GreatDBBackupRecordInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.GreatDBBackupRecordLister
}

type greatDBBackupRecordInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewGreatDBBackupRecordInformer constructs a new informer for GreatDBBackupRecord type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewGreatDBBackupRecordInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredGreatDBBackupRecordInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredGreatDBBackupRecordInformer constructs a new informer for GreatDBBackupRecord type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredGreatDBBackupRecordInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.GreatdbV1alpha1().GreatDBBackupRecords(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.GreatdbV1alpha1().GreatDBBackupRecords(namespace).Watch(context.TODO(), options)
			},
		},
		&greatdbv1alpha1.GreatDBBackupRecord{},
		resyncPeriod,
		indexers,
	)
}

func (f *greatDBBackupRecordInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredGreatDBBackupRecordInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *greatDBBackupRecordInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&greatdbv1alpha1.GreatDBBackupRecord{}, f.defaultInformer)
}

func (f *greatDBBackupRecordInformer) Lister() v1alpha1.GreatDBBackupRecordLister {
	return v1alpha1.NewGreatDBBackupRecordLister(f.Informer().GetIndexer())
}