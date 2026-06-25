package kubectl

import (
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func i32(n int32) *int32 { return &n }

func TestDeploymentReady(t *testing.T) {
	base := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Generation: 2},
		Spec:       appsv1.DeploymentSpec{Replicas: i32(3)},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 2, UpdatedReplicas: 3, ReadyReplicas: 3, AvailableReplicas: 3,
		},
	}
	if !deploymentReady(&base) {
		t.Error("fully rolled-out deployment should be ready")
	}

	notReady := *base.DeepCopy()
	notReady.Status.ReadyReplicas = 2
	if deploymentReady(&notReady) {
		t.Error("deployment with readyReplicas < desired should not be ready")
	}

	stale := *base.DeepCopy()
	stale.Status.ObservedGeneration = 1
	if deploymentReady(&stale) {
		t.Error("deployment whose controller has not observed the latest generation should not be ready")
	}
}

func TestStatefulSetReady(t *testing.T) {
	ready := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Generation: 1},
		Spec:       appsv1.StatefulSetSpec{Replicas: i32(2)},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1, UpdatedReplicas: 2, ReadyReplicas: 2,
			CurrentRevision: "rev2", UpdateRevision: "rev2",
		},
	}
	if !statefulSetReady(&ready) {
		t.Error("converged statefulset should be ready")
	}
	updating := *ready.DeepCopy()
	updating.Status.CurrentRevision = "rev1"
	if statefulSetReady(&updating) {
		t.Error("statefulset mid rolling-update should not be ready")
	}
}

func TestJobReady(t *testing.T) {
	complete := batchv1.Job{Spec: batchv1.JobSpec{Completions: i32(2)}, Status: batchv1.JobStatus{Succeeded: 2}}
	if ready, failed := jobReady(&complete); !ready || failed {
		t.Errorf("completed parallel job: ready=%v failed=%v", ready, failed)
	}

	partial := batchv1.Job{Spec: batchv1.JobSpec{Completions: i32(2)}, Status: batchv1.JobStatus{Succeeded: 1}}
	if ready, _ := jobReady(&partial); ready {
		t.Error("parallel job with completions>succeeded should not be ready")
	}

	defaulted := batchv1.Job{Status: batchv1.JobStatus{Succeeded: 1}} // nil completions => 1
	if ready, _ := jobReady(&defaulted); !ready {
		t.Error("job with default completions and one success should be ready")
	}

	failed := batchv1.Job{Status: batchv1.JobStatus{
		Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}},
	}}
	if _, isFailed := jobReady(&failed); !isFailed {
		t.Error("a Failed job condition should be detected")
	}
}

func TestAllReady(t *testing.T) {
	items := []appsv1.Deployment{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "a", Generation: 1},
			Spec:       appsv1.DeploymentSpec{Replicas: i32(1)},
			Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1},
		},
	}
	name := func(d *appsv1.Deployment) string { return d.Name }
	ready := func(d *appsv1.Deployment) (bool, error) { return deploymentReady(d), nil }

	if ok, err := allReady([]string{"a"}, items, name, ready); err != nil || !ok {
		t.Errorf("present ready item: ok=%v err=%v", ok, err)
	}
	if ok, _ := allReady([]string{"a", "missing"}, items, name, ready); ok {
		t.Error("a requested name absent from the cluster should not be considered ready")
	}
}

func TestAreDeploymentsReadyDecodesJSON(t *testing.T) {
	orig := runKubectl
	defer func() { runKubectl = orig }()
	runKubectl = func(_ context.Context, _ []string) ([]byte, error) {
		return []byte(`{"items":[{"metadata":{"name":"a","generation":1},"spec":{"replicas":1},"status":{"observedGeneration":1,"updatedReplicas":1,"readyReplicas":1,"availableReplicas":1}}]}`), nil
	}
	ok, err := AreDeploymentsReady(context.Background(), []string{"a"}, "ns", false)
	if err != nil || !ok {
		t.Fatalf("expected ready, got ok=%v err=%v", ok, err)
	}
}

func TestAreDeploymentsReadyPropagatesError(t *testing.T) {
	orig := runKubectl
	defer func() { runKubectl = orig }()
	runKubectl = func(_ context.Context, _ []string) ([]byte, error) { return nil, errors.New("boom") }
	if _, err := AreDeploymentsReady(context.Background(), []string{"a"}, "ns", false); err == nil {
		t.Fatal("expected the kubectl error to propagate")
	}
}
