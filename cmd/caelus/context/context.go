/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 *
 * You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package context

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

// CaelusContext stores k8s client and factory
type CaelusContext struct {
	Master                  string
	Kubeconfig              string
	NodeName                string
	kubeClient              clientset.Interface
	nodeFactory, podFactory informers.SharedInformerFactory
}

const (
	nodeNameField      = "metadata.name"
	specNodeNameField  = "spec.nodeName"
	statusPhaseFiled   = "status.phase"
	informerSyncPeriod = time.Minute
)

// lazyInit build kubernetes client
func (c *CaelusContext) lazyInit() {
	if c.kubeClient != nil {
		return
	}
	kubeconfig, err := clientcmd.BuildConfigFromFlags(c.Master, c.Kubeconfig)
	if err != nil {
		klog.Warning(err)
		klog.Warning("fall back to creating fake kube-client")
		// create a fake client to test caelus without k8s
		c.kubeClient = fake.NewSimpleClientset()
	} else {
		c.kubeClient = clientset.NewForConfigOrDie(kubeconfig)
	}
}

// GetKubeClient returns k8s client
func (c *CaelusContext) GetKubeClient() clientset.Interface {
	c.lazyInit()
	return c.kubeClient
}

// GetPodFactory returns pod factory
func (c *CaelusContext) GetPodFactory() informers.SharedInformerFactory {
	if c.podFactory == nil {
		// pod informer no need to list pods with finished state, such as succeeded and failed
		c.podFactory = informers.NewSharedInformerFactoryWithOptions(c.GetKubeClient(), informerSyncPeriod,
			informers.WithTweakListOptions(func(options *metav1.ListOptions) {
				options.FieldSelector = fields.AndSelectors(fields.OneTermEqualSelector(specNodeNameField, c.NodeName),
					fields.OneTermNotEqualSelector(statusPhaseFiled, "Succeeded"),
					fields.OneTermNotEqualSelector(statusPhaseFiled, "Failed")).String()
			}))
	}
	return c.podFactory
}

// GetNodeFactory returns node factory
func (c *CaelusContext) GetNodeFactory() informers.SharedInformerFactory {
	if c.nodeFactory == nil {
		c.nodeFactory = informers.NewSharedInformerFactoryWithOptions(c.GetKubeClient(), informerSyncPeriod,
			informers.WithTweakListOptions(func(options *metav1.ListOptions) {
				options.FieldSelector = fields.OneTermEqualSelector(nodeNameField, c.NodeName).String()
			}))
	}
	return c.nodeFactory
}

// Name module name
func (c *CaelusContext) Name() string {
	return "ModuleContext"
}

// Run starts k8s informers
func (c *CaelusContext) Run(stop <-chan struct{}) {
	if c.podFactory != nil {
		c.podFactory.Start(stop)
		c.podFactory.WaitForCacheSync(stop)
	}
	if c.nodeFactory != nil {
		c.nodeFactory.Start(stop)
		c.nodeFactory.WaitForCacheSync(stop)
	}
}
