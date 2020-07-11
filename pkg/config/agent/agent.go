/*
 * Copyright 2020 The Multicluster-Scheduler Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package agent

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"admiralty.io/multicluster-scheduler/pkg/apis/multicluster/v1alpha1"
	"admiralty.io/multicluster-scheduler/pkg/generated/clientset/versioned"
)

type Config struct {
	Targets []Target
}

type Target struct {
	Name         string
	ClientConfig *rest.Config
	Self         bool // optimization to re-use clients, informers, etc.
	Namespace    string
}

func (t Target) GetKey() string {
	if t.Namespace == "" {
		return fmt.Sprintf("cluster-%s", t.Name)
	} else {
		return fmt.Sprintf("namespace-%s-%s", t.Namespace, t.Name)
	}
}

// until we watch targets at runtime, we can already load them from objects at startup
func NewFromCRD(ctx context.Context) Config {
	cfg := config.GetConfigOrDie()

	customClient, err := versioned.NewForConfig(cfg)
	utilruntime.Must(err)

	k, err := kubernetes.NewForConfig(cfg)
	utilruntime.Must(err)

	agentCfg := Config{}

	cl, err := customClient.MulticlusterV1alpha1().ClusterTargets().List(ctx, metav1.ListOptions{})
	utilruntime.Must(err)
	for _, t := range cl.Items {
		addClusterTarget(ctx, k, &agentCfg, t)
	}

	l, err := customClient.MulticlusterV1alpha1().Targets(corev1.NamespaceAll).List(ctx, metav1.ListOptions{})
	utilruntime.Must(err)
	for _, t := range l.Items {
		addTarget(ctx, k, &agentCfg, t)
	}

	return agentCfg
}

func addClusterTarget(ctx context.Context, k *kubernetes.Clientset, agentCfg *Config, t v1alpha1.ClusterTarget) {
	if t.Spec.Self == (t.Spec.KubeconfigSecret != nil) {
		utilruntime.Must(fmt.Errorf("self XOR kubeconfigSecret != nil"))
		// TODO validating webhook to catch user error upstream
	}
	var cfg *rest.Config
	if kcfg := t.Spec.KubeconfigSecret; kcfg != nil {
		cfg = getConfigFromKubeconfigSecretOrDie(ctx, k, kcfg.Namespace, kcfg.Name, kcfg.Key, kcfg.Context)
	} else {
		cfg = config.GetConfigOrDie()
	}

	c := Target{Name: t.Name, ClientConfig: cfg, Namespace: corev1.NamespaceAll}
	agentCfg.Targets = append(agentCfg.Targets, c)
}

func addTarget(ctx context.Context, k *kubernetes.Clientset, agentCfg *Config, t v1alpha1.Target) {
	if t.Spec.Self == (t.Spec.KubeconfigSecret != nil) {
		utilruntime.Must(fmt.Errorf("self XOR kubeconfigSecret != nil"))
		// TODO validating webhook to catch user error upstream
	}
	var cfg *rest.Config
	if kcfg := t.Spec.KubeconfigSecret; kcfg != nil {
		cfg = getConfigFromKubeconfigSecretOrDie(ctx, k, t.Namespace, kcfg.Name, kcfg.Key, kcfg.Context)
	} else {
		cfg = config.GetConfigOrDie()
	}

	c := Target{Name: t.Name, ClientConfig: cfg, Namespace: t.Namespace}
	agentCfg.Targets = append(agentCfg.Targets, c)
}

func getConfigFromKubeconfigSecretOrDie(ctx context.Context, k *kubernetes.Clientset, namespace, name, key, context string) *rest.Config {
	if key == "" {
		key = "config"
	}

	s, err := k.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	utilruntime.Must(err)

	cfg0, err := clientcmd.Load(s.Data[key])
	utilruntime.Must(err)

	cfg1 := clientcmd.NewDefaultClientConfig(*cfg0, &clientcmd.ConfigOverrides{CurrentContext: context})

	cfg2, err := cfg1.ClientConfig()
	utilruntime.Must(err)

	return cfg2
}
