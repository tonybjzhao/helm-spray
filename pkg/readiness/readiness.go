/*
(c) Copyright 2018, Gemalto. All rights reserved.
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

// Package readiness reports when the workloads created by a release have become
// ready. It queries the Kubernetes API directly through the embedded client-go
// client — no external kubectl binary is required — and evaluates Deployments,
// StatefulSets, DaemonSets and Jobs from their typed status. The clientset is
// built once from the ambient kubeconfig (the same default loading rules as helm
// and kubectl) and honours the connection settings helm exports to its plugins
// (HELM_KUBECONTEXT, HELM_KUBEAPISERVER, HELM_KUBETOKEN, ...), so readiness is
// checked against the same cluster, context and identity that helm deploys to.
package readiness

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ThalesGroup/helm-spray/v5/internal/log"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// clientTimeout bounds each Kubernetes API call so a hung apiserver cannot block
// readiness polling past its own deadline.
const clientTimeout = 30 * time.Second

// client returns the Kubernetes clientset used for readiness checks. It is a
// package variable so tests can substitute a fake clientset.
var client = defaultClient

var (
	clientOnce sync.Once
	clientset  kubernetes.Interface
	clientErr  error
)

// defaultClient builds (once) a clientset from the ambient kubeconfig, resolving
// the config the same way helm and kubectl do (KUBECONFIG, ~/.kube/config, the
// in-cluster service account, and the current context).
func defaultClient() (kubernetes.Interface, error) {
	clientOnce.Do(func() {
		rules := clientcmd.NewDefaultClientConfigLoadingRules()
		cfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, helmKubeOverrides())
		restCfg, err := cfg.ClientConfig()
		if err != nil {
			clientErr = fmt.Errorf("loading kubeconfig: %w", err)
			return
		}
		if restCfg.Timeout == 0 {
			restCfg.Timeout = clientTimeout
		}
		clientset, clientErr = kubernetes.NewForConfig(restCfg)
	})
	return clientset, clientErr
}

// helmKubeOverrides mirrors the kube connection settings helm exports to its
// plugins (see helm.sh/helm cli/environment.go), so readiness is checked against
// the same cluster, context and identity helm deploys to — in particular helm's
// --kube-context — rather than the kubeconfig's default current-context.
func helmKubeOverrides() *clientcmd.ConfigOverrides {
	o := &clientcmd.ConfigOverrides{}
	if v := os.Getenv("HELM_KUBECONTEXT"); v != "" {
		o.CurrentContext = v
	}
	if v := os.Getenv("HELM_KUBEAPISERVER"); v != "" {
		o.ClusterInfo.Server = v
	}
	if v := os.Getenv("HELM_KUBETOKEN"); v != "" {
		o.AuthInfo.Token = v
	}
	if v := os.Getenv("HELM_KUBEASUSER"); v != "" {
		o.AuthInfo.Impersonate = v
	}
	if v := os.Getenv("HELM_KUBECAFILE"); v != "" {
		o.ClusterInfo.CertificateAuthority = v
	}
	return o
}

// AreDeploymentsReady reports whether every named Deployment in the namespace has
// completed its rollout. A name that does not (yet) exist is treated as not ready
// so the caller keeps waiting.
func AreDeploymentsReady(ctx context.Context, names []string, namespace string, debug bool) (bool, error) {
	cs, err := client()
	if err != nil {
		return false, err
	}
	logQuery(debug, "deployments", namespace)
	list, err := cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("listing deployments in namespace %q: %w", namespace, err)
	}
	return allReady(names, list.Items,
		func(d *appsv1.Deployment) string { return d.Name },
		func(d *appsv1.Deployment) (bool, error) { return deploymentReady(d), nil })
}

// AreStatefulSetsReady reports whether every named StatefulSet has completed its
// rollout.
func AreStatefulSetsReady(ctx context.Context, names []string, namespace string, debug bool) (bool, error) {
	cs, err := client()
	if err != nil {
		return false, err
	}
	logQuery(debug, "statefulsets", namespace)
	list, err := cs.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("listing statefulsets in namespace %q: %w", namespace, err)
	}
	return allReady(names, list.Items,
		func(s *appsv1.StatefulSet) string { return s.Name },
		func(s *appsv1.StatefulSet) (bool, error) { return statefulSetReady(s), nil })
}

// AreDaemonSetsReady reports whether every named DaemonSet has rolled out to all
// scheduled nodes.
func AreDaemonSetsReady(ctx context.Context, names []string, namespace string, debug bool) (bool, error) {
	cs, err := client()
	if err != nil {
		return false, err
	}
	logQuery(debug, "daemonsets", namespace)
	list, err := cs.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("listing daemonsets in namespace %q: %w", namespace, err)
	}
	return allReady(names, list.Items,
		func(d *appsv1.DaemonSet) string { return d.Name },
		func(d *appsv1.DaemonSet) (bool, error) { return daemonSetReady(d), nil })
}

// AreJobsReady reports whether every named Job has reached its required number of
// successful completions. If a Job has definitively failed it returns an error so
// the caller fails fast instead of waiting out the whole timeout.
func AreJobsReady(ctx context.Context, names []string, namespace string, debug bool) (bool, error) {
	cs, err := client()
	if err != nil {
		return false, err
	}
	logQuery(debug, "jobs", namespace)
	list, err := cs.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("listing jobs in namespace %q: %w", namespace, err)
	}
	return allReady(names, list.Items,
		func(j *batchv1.Job) string { return j.Name },
		func(j *batchv1.Job) (bool, error) {
			ready, failed := jobReady(j)
			if failed {
				return false, fmt.Errorf("job %q failed", j.Name)
			}
			return ready, nil
		})
}

func logQuery(debug bool, kind, namespace string) {
	if debug {
		log.Info(3, "querying %s in namespace %q via the Kubernetes API", kind, namespace)
	}
}

func deploymentReady(d *appsv1.Deployment) bool {
	desired := desiredReplicas(d.Spec.Replicas)
	return d.Status.ObservedGeneration >= d.Generation &&
		d.Status.UpdatedReplicas >= desired &&
		d.Status.ReadyReplicas >= desired &&
		d.Status.AvailableReplicas >= desired
}

func statefulSetReady(s *appsv1.StatefulSet) bool {
	desired := desiredReplicas(s.Spec.Replicas)
	if s.Status.ObservedGeneration < s.Generation {
		return false
	}
	if s.Status.ReadyReplicas < desired {
		return false
	}
	// With the OnDelete update strategy the controller does not roll pods
	// automatically: it waits for an operator to delete them. The updated-replica
	// count and the current/update revisions therefore never converge on their
	// own after a spec change, so gating on them would wait out the whole timeout.
	// Readiness for OnDelete is based on ready replicas alone.
	if s.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType {
		return true
	}
	if s.Status.UpdatedReplicas < desired {
		return false
	}
	// During a RollingUpdate the current and update revisions differ until the
	// rollout completes.
	if s.Status.UpdateRevision != "" && s.Status.CurrentRevision != s.Status.UpdateRevision {
		return false
	}
	return true
}

func daemonSetReady(d *appsv1.DaemonSet) bool {
	return d.Status.ObservedGeneration >= d.Generation &&
		d.Status.UpdatedNumberScheduled >= d.Status.DesiredNumberScheduled &&
		d.Status.NumberReady >= d.Status.DesiredNumberScheduled &&
		d.Status.NumberUnavailable == 0
}

// jobReady reports whether a Job has reached its required successful completions,
// and separately whether it has definitively failed.
func jobReady(j *batchv1.Job) (ready bool, failed bool) {
	for _, c := range j.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return false, true
		}
	}
	completions := int32(1)
	if j.Spec.Completions != nil {
		completions = *j.Spec.Completions
	}
	return j.Status.Succeeded >= completions, false
}

func desiredReplicas(r *int32) int32 {
	if r == nil {
		return 1
	}
	return *r
}

// allReady returns true only if every requested name is present among items and
// satisfies ready. A requested name that is absent yields (false, nil) so the
// caller keeps polling until the workload appears.
func allReady[T any](names []string, items []T, name func(*T) string, ready func(*T) (bool, error)) (bool, error) {
	byName := make(map[string]*T, len(items))
	for i := range items {
		byName[name(&items[i])] = &items[i]
	}
	for _, n := range names {
		match, ok := byName[n]
		if !ok {
			return false, nil
		}
		readyOK, err := ready(match)
		if err != nil {
			return false, err
		}
		if !readyOK {
			return false, nil
		}
	}
	return true, nil
}
