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
// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "greatdb-operator/pkg/apis/greatdb/v1alpha1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// GreatDBPaxosLister helps list GreatDBPaxoses.
// All objects returned here must be treated as read-only.
type GreatDBPaxosLister interface {
	// List lists all GreatDBPaxoses in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.GreatDBPaxos, err error)
	// GreatDBPaxoses returns an object that can list and get GreatDBPaxoses.
	GreatDBPaxoses(namespace string) GreatDBPaxosNamespaceLister
	GreatDBPaxosListerExpansion
}

// greatDBPaxosLister implements the GreatDBPaxosLister interface.
type greatDBPaxosLister struct {
	indexer cache.Indexer
}

// NewGreatDBPaxosLister returns a new GreatDBPaxosLister.
func NewGreatDBPaxosLister(indexer cache.Indexer) GreatDBPaxosLister {
	return &greatDBPaxosLister{indexer: indexer}
}

// List lists all GreatDBPaxoses in the indexer.
func (s *greatDBPaxosLister) List(selector labels.Selector) (ret []*v1alpha1.GreatDBPaxos, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.GreatDBPaxos))
	})
	return ret, err
}

// GreatDBPaxoses returns an object that can list and get GreatDBPaxoses.
func (s *greatDBPaxosLister) GreatDBPaxoses(namespace string) GreatDBPaxosNamespaceLister {
	return greatDBPaxosNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// GreatDBPaxosNamespaceLister helps list and get GreatDBPaxoses.
// All objects returned here must be treated as read-only.
type GreatDBPaxosNamespaceLister interface {
	// List lists all GreatDBPaxoses in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.GreatDBPaxos, err error)
	// Get retrieves the GreatDBPaxos from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.GreatDBPaxos, error)
	GreatDBPaxosNamespaceListerExpansion
}

// greatDBPaxosNamespaceLister implements the GreatDBPaxosNamespaceLister
// interface.
type greatDBPaxosNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all GreatDBPaxoses in the indexer for a given namespace.
func (s greatDBPaxosNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.GreatDBPaxos, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.GreatDBPaxos))
	})
	return ret, err
}

// Get retrieves the GreatDBPaxos from the indexer for a given namespace and name.
func (s greatDBPaxosNamespaceLister) Get(name string) (*v1alpha1.GreatDBPaxos, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("greatdbpaxos"), name)
	}
	return obj.(*v1alpha1.GreatDBPaxos), nil
}
