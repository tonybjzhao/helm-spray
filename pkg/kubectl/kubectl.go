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

// Package kubectl wraps the kubectl command-line interface to determine when the
// workloads created by a release have become ready. It queries each workload
// kind once as JSON and evaluates readiness from the decoded Kubernetes objects,
// rather than embedding workload names into go-templates.
package kubectl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/ThalesGroup/helm-spray/v4/internal/log"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

// AreDeploymentsReady reports whether every named Deployment in the namespace
// has completed its rollout. A name that does not (yet) exist is treated as not
// ready so the caller keeps waiting.
func AreDeploymentsReady(ctx context.Context, names []string, namespace string, debug bool) (bool, error) {
	var list appsv1.DeploymentList
	if err := getList(ctx, namespace, "deployments", debug, &list); err != nil {
		return false, err
	}
	return allReady(names, list.Items,
		func(d *appsv1.Deployment) string { return d.Name },
		func(d *appsv1.Deployment) (bool, error) { return deploymentReady(d), nil })
}

// AreStatefulSetsReady reports whether every named StatefulSet has completed its
// rollout.
func AreStatefulSetsReady(ctx context.Context, names []string, namespace string, debug bool) (bool, error) {
	var list appsv1.StatefulSetList
	if err := getList(ctx, namespace, "statefulsets", debug, &list); err != nil {
		return false, err
	}
	return allReady(names, list.Items,
		func(s *appsv1.StatefulSet) string { return s.Name },
		func(s *appsv1.StatefulSet) (bool, error) { return statefulSetReady(s), nil })
}

// AreDaemonSetsReady reports whether every named DaemonSet has rolled out to all
// scheduled nodes.
func AreDaemonSetsReady(ctx context.Context, names []string, namespace string, debug bool) (bool, error) {
	var list appsv1.DaemonSetList
	if err := getList(ctx, namespace, "daemonsets", debug, &list); err != nil {
		return false, err
	}
	return allReady(names, list.Items,
		func(d *appsv1.DaemonSet) string { return d.Name },
		func(d *appsv1.DaemonSet) (bool, error) { return daemonSetReady(d), nil })
}

// AreJobsReady reports whether every named Job has reached its required number of
// successful completions. If a Job has definitively failed it returns an error so
// the caller fails fast instead of waiting out the whole timeout.
func AreJobsReady(ctx context.Context, names []string, namespace string, debug bool) (bool, error) {
	var list batchv1.JobList
	if err := getList(ctx, namespace, "jobs", debug, &list); err != nil {
		return false, err
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
	if s.Status.UpdatedReplicas < desired || s.Status.ReadyReplicas < desired {
		return false
	}
	// During a rolling update the current and update revisions differ.
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
	for _, n := range names {
		var match *T
		for i := range items {
			if name(&items[i]) == n {
				match = &items[i]
				break
			}
		}
		if match == nil {
			return false, nil
		}
		ok, err := ready(match)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// getList runs "kubectl get <resource> -o json" in the namespace and decodes the
// result into the provided typed list.
func getList(ctx context.Context, namespace, resource string, debug bool, into any) error {
	args := []string{"--namespace", namespace, "get", resource, "-o", "json"}
	if debug {
		log.Info(3, "running kubectl command: %v", args)
	}
	cmd := exec.CommandContext(ctx, "kubectl", args...) // #nosec G204 -- fixed "kubectl get" subcommand; namespace/resource are argv elements, not a shell
	out := &bytes.Buffer{}
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running kubectl get %s in namespace %q: %w", resource, namespace, err)
	}
	if debug {
		log.Info(3, "kubectl output: %s", out.String())
	}
	if err := json.Unmarshal(out.Bytes(), into); err != nil {
		return fmt.Errorf("parsing kubectl get %s output: %w", resource, err)
	}
	return nil
}
