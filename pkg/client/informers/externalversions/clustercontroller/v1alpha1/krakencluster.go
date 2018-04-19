/*
Copyright 2018 Samsung CNCT.

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
	time "time"

	clustercontroller_v1alpha1 "github.com/samsung-cnct/cluster-controller/pkg/apis/clustercontroller/v1alpha1"
	versioned "github.com/samsung-cnct/cluster-controller/pkg/client/clientset/versioned"
	internalinterfaces "github.com/samsung-cnct/cluster-controller/pkg/client/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/samsung-cnct/cluster-controller/pkg/client/listers/clustercontroller/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// KrakenClusterInformer provides access to a shared informer and lister for
// KrakenClusters.
type KrakenClusterInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.KrakenClusterLister
}

type krakenClusterInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewKrakenClusterInformer constructs a new informer for KrakenCluster type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewKrakenClusterInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredKrakenClusterInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredKrakenClusterInformer constructs a new informer for KrakenCluster type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredKrakenClusterInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.SamsungV1alpha1().KrakenClusters(namespace).List(options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.SamsungV1alpha1().KrakenClusters(namespace).Watch(options)
			},
		},
		&clustercontroller_v1alpha1.KrakenCluster{},
		resyncPeriod,
		indexers,
	)
}

func (f *krakenClusterInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredKrakenClusterInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *krakenClusterInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&clustercontroller_v1alpha1.KrakenCluster{}, f.defaultInformer)
}

func (f *krakenClusterInformer) Lister() v1alpha1.KrakenClusterLister {
	return v1alpha1.NewKrakenClusterLister(f.Informer().GetIndexer())
}