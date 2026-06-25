package helmspray

import (
	"context"

	"github.com/ThalesGroup/helm-spray/v4/pkg/helm"
	"github.com/ThalesGroup/helm-spray/v4/pkg/kubectl"
)

// HelmClient performs the helm operations the orchestrator needs. It is an
// interface so the weight-loop / upgrade orchestration can be unit-tested with a
// fake, without a real helm binary or cluster.
type HelmClient interface {
	List(ctx context.Context, namespace string, debug bool) (map[string]helm.Release, error)
	Upgrade(ctx context.Context, req helm.UpgradeRequest) (helm.UpgradedRelease, error)
}

// ReadinessChecker reports whether the workloads created by a release have
// become ready. It is an interface for the same testability reason as
// HelmClient.
type ReadinessChecker interface {
	DeploymentsReady(ctx context.Context, names []string, namespace string, debug bool) (bool, error)
	StatefulSetsReady(ctx context.Context, names []string, namespace string, debug bool) (bool, error)
	DaemonSetsReady(ctx context.Context, names []string, namespace string, debug bool) (bool, error)
	JobsReady(ctx context.Context, names []string, namespace string, debug bool) (bool, error)
}

// execHelmClient is the default HelmClient, backed by the helm CLI wrapper.
type execHelmClient struct{}

func (execHelmClient) List(ctx context.Context, namespace string, debug bool) (map[string]helm.Release, error) {
	return helm.List(ctx, namespace, debug)
}

func (execHelmClient) Upgrade(ctx context.Context, req helm.UpgradeRequest) (helm.UpgradedRelease, error) {
	return helm.UpgradeWithValues(ctx, req)
}

// execReadinessChecker is the default ReadinessChecker, backed by the kubectl
// wrapper.
type execReadinessChecker struct{}

func (execReadinessChecker) DeploymentsReady(ctx context.Context, names []string, namespace string, debug bool) (bool, error) {
	return kubectl.AreDeploymentsReady(ctx, names, namespace, debug)
}

func (execReadinessChecker) StatefulSetsReady(ctx context.Context, names []string, namespace string, debug bool) (bool, error) {
	return kubectl.AreStatefulSetsReady(ctx, names, namespace, debug)
}

func (execReadinessChecker) DaemonSetsReady(ctx context.Context, names []string, namespace string, debug bool) (bool, error) {
	return kubectl.AreDaemonSetsReady(ctx, names, namespace, debug)
}

func (execReadinessChecker) JobsReady(ctx context.Context, names []string, namespace string, debug bool) (bool, error) {
	return kubectl.AreJobsReady(ctx, names, namespace, debug)
}
