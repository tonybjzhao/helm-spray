package readiness

import (
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func i32(n int32) *int32 { return &n }

// withFakeClient swaps the package clientset for a fake seeded with objs, and
// returns a restore function for use with defer.
func withFakeClient(t *testing.T, objs ...runtime.Object) func() {
	t.Helper()
	orig := client
	cs := fake.NewClientset(objs...)
	client = func() (kubernetes.Interface, error) { return cs, nil }
	return func() { client = orig }
}

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

func TestStatefulSetReadyOnDelete(t *testing.T) {
	// With the OnDelete update strategy the controller never rolls pods on its
	// own, so revisions and updated-replica counts do not converge after a spec
	// change. Readiness must rely on ready replicas alone (issue #58), otherwise
	// the wait would never finish.
	onDelete := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Generation: 2},
		Spec: appsv1.StatefulSetSpec{
			Replicas:       i32(2),
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{Type: appsv1.OnDeleteStatefulSetStrategyType},
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 2, ReadyReplicas: 2, UpdatedReplicas: 1,
			CurrentRevision: "rev1", UpdateRevision: "rev2",
		},
	}
	if !statefulSetReady(&onDelete) {
		t.Error("OnDelete statefulset with all replicas ready should be ready despite unconverged revisions")
	}
	notReady := *onDelete.DeepCopy()
	notReady.Status.ReadyReplicas = 1
	if statefulSetReady(&notReady) {
		t.Error("OnDelete statefulset with missing ready replicas should not be ready")
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

func TestDaemonSetReady(t *testing.T) {
	ready := appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Generation: 1},
		Status: appsv1.DaemonSetStatus{
			ObservedGeneration:     1,
			DesiredNumberScheduled: 3,
			UpdatedNumberScheduled: 3,
			NumberReady:            3,
			NumberUnavailable:      0,
		},
	}
	if !daemonSetReady(&ready) {
		t.Error("a fully rolled-out daemonset should be ready")
	}
	stale := *ready.DeepCopy()
	stale.Status.ObservedGeneration = 0
	if daemonSetReady(&stale) {
		t.Error("a daemonset whose controller has not observed the latest generation should not be ready")
	}
	rolling := *ready.DeepCopy()
	rolling.Status.UpdatedNumberScheduled = 2
	if daemonSetReady(&rolling) {
		t.Error("a daemonset mid-rollout should not be ready")
	}
	unavailable := *ready.DeepCopy()
	unavailable.Status.NumberUnavailable = 1
	if daemonSetReady(&unavailable) {
		t.Error("a daemonset with unavailable pods should not be ready")
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

func TestAreDeploymentsReadyViaAPI(t *testing.T) {
	defer withFakeClient(t, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Generation: 1},
		Spec:       appsv1.DeploymentSpec{Replicas: i32(1)},
		Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1},
	})()
	ok, err := AreDeploymentsReady(context.Background(), []string{"a"}, "ns", false)
	if err != nil || !ok {
		t.Fatalf("expected ready, got ok=%v err=%v", ok, err)
	}
}

func TestAreStatefulSetsReadyViaAPI(t *testing.T) {
	defer withFakeClient(t, &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "ns", Generation: 1},
		Spec:       appsv1.StatefulSetSpec{Replicas: i32(1)},
		Status:     appsv1.StatefulSetStatus{ObservedGeneration: 1, UpdatedReplicas: 1, ReadyReplicas: 1, CurrentRevision: "r", UpdateRevision: "r"},
	})()
	ok, err := AreStatefulSetsReady(context.Background(), []string{"s"}, "ns", false)
	if err != nil || !ok {
		t.Fatalf("expected ready, got ok=%v err=%v", ok, err)
	}
}

func TestAreDaemonSetsReadyViaAPI(t *testing.T) {
	defer withFakeClient(t, &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "ns", Generation: 1},
		Status:     appsv1.DaemonSetStatus{ObservedGeneration: 1, DesiredNumberScheduled: 1, UpdatedNumberScheduled: 1, NumberReady: 1},
	})()
	ok, err := AreDaemonSetsReady(context.Background(), []string{"d"}, "ns", false)
	if err != nil || !ok {
		t.Fatalf("expected ready, got ok=%v err=%v", ok, err)
	}
}

func TestAreJobsReadyViaAPI(t *testing.T) {
	defer withFakeClient(t, &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "j", Namespace: "ns"},
		Spec:       batchv1.JobSpec{Completions: i32(1)},
		Status:     batchv1.JobStatus{Succeeded: 1},
	})()
	ok, err := AreJobsReady(context.Background(), []string{"j"}, "ns", false)
	if err != nil || !ok {
		t.Fatalf("expected ready, got ok=%v err=%v", ok, err)
	}
}

func TestAreJobsReadyFailsFast(t *testing.T) {
	defer withFakeClient(t, &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "j", Namespace: "ns"},
		Status:     batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}},
	})()
	if _, err := AreJobsReady(context.Background(), []string{"j"}, "ns", false); err == nil {
		t.Fatal("a failed job should surface an error so the caller fails fast")
	}
}

func TestReadinessClientErrorPropagates(t *testing.T) {
	orig := client
	defer func() { client = orig }()
	client = func() (kubernetes.Interface, error) { return nil, errors.New("boom") }
	if _, err := AreDeploymentsReady(context.Background(), []string{"a"}, "ns", false); err == nil {
		t.Fatal("expected the client error to propagate")
	}
}
