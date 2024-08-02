package fsm

import (
	"context"
	"slices"

	authenticationv1alpha1 "github.com/gardener/gardener/pkg/apis/authentication/v1alpha1"
	gardener_api "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	imv1 "github.com/kyma-project/infrastructure-manager/api/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	labelsClusterRoleBindings = map[string]string{
		"app":                                   "kyma",
		"reconciler.kyma-project.io/managed-by": "kim",
	}
)

func getShootClientWithAdmin(ctx context.Context,
	adminKubeconfigClient client.SubResourceClient,
	shoot *gardener_api.Shoot) (client.Client, error) {
	// request for admin kubeconfig with low expiration timeout
	var req authenticationv1alpha1.AdminKubeconfigRequest
	if err := adminKubeconfigClient.Create(ctx, shoot, &req); err != nil {
		return nil, err
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(req.Status.Kubeconfig)
	if err != nil {
		return nil, err
	}

	shootClientWithAdmin, err := client.New(restConfig, client.Options{})
	if err != nil {
		return nil, err
	}

	return shootClientWithAdmin, nil
}

func isRBACUserKind(s rbacv1.Subject) bool {
	return s.Kind == rbacv1.UserKind &&
		s.APIGroup == rbacv1.GroupName
}

func getRemoved(crbs []rbacv1.ClusterRoleBinding, admins []string) (removed []rbacv1.ClusterRoleBinding) {
	// iterate over cluster role bindings to find out removed administrators
	for _, crb := range crbs {
		if !labels.Set(crb.Labels).AsSelector().Matches(labels.Set(labelsClusterRoleBindings)) {
			// cluster role binding is not controlled by KIM
			continue
		}

		index := slices.IndexFunc(crb.Subjects, isRBACUserKind)
		if index < 0 {
			// cluster role binding does not contain user subject
			continue
		}

		subjectUserName := crb.Subjects[index].Name
		if slices.Contains(admins, subjectUserName) {
			continue
		}
		// administrator was removed
		removed = append(removed, crb)
	}

	return removed
}

var newContainsAdmin = func(admin string) func(rbacv1.ClusterRoleBinding) bool {
	return func(r rbacv1.ClusterRoleBinding) bool {
		for _, subject := range r.Subjects {
			if !isRBACUserKind(subject) || subject.Name != admin {
				continue
			}
			// admin found
			return true
		}
		// admin not found in the slice
		return false
	}
}

func getMissing(crbs []rbacv1.ClusterRoleBinding, admins []string) (missing []rbacv1.ClusterRoleBinding) {
	for _, admin := range admins {
		containsAdmin := newContainsAdmin(admin)
		if slices.ContainsFunc(crbs, containsAdmin) {
			continue
		}
		crb := toAdminClusterRoleBinding(admin)
		missing = append(missing, crb)
	}

	return missing
}

func toAdminClusterRoleBinding(name string) rbacv1.ClusterRoleBinding {
	return rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "admin-",
			Labels:       labelsClusterRoleBindings,
		},
		Subjects: []rbacv1.Subject{{
			Kind:     rbacv1.UserKind,
			Name:     name,
			APIGroup: rbacv1.GroupName,
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
	}
}

var newDelCRBs = func(ctx context.Context, shootClient client.Client, crbs []rbacv1.ClusterRoleBinding) func() error {
	return func() error {
		for _, crb := range crbs {
			if err := shootClient.Delete(ctx, &crb); err != nil {
				return err
			}
		}
		return nil
	}
}

var newAddCRBs = func(ctx context.Context, shootClient client.Client, crbs []rbacv1.ClusterRoleBinding) func() error {
	return func() error {
		for _, crb := range crbs {
			if err := shootClient.Create(ctx, &crb); err != nil {
				return err
			}
		}
		return nil
	}
}

func updateCRBApplyFailed(rt *imv1.Runtime) {
	rt.UpdateStatePending(
		imv1.ConditionTypeRuntimeConfigured,
		imv1.ConditionReasonConfigurationErr,
		string(metav1.ConditionFalse),
		"failed to update kubeconfig admin access",
	)
}

func sFnApplyClusterRoleBindings(ctx context.Context, m *fsm, s *systemState) (stateFn, *ctrl.Result, error) {
	// prepare subresource client to request admin kubeconfig
	srscClient := m.ShootClient.SubResource("adminkubeconfig")
	shootAdminClient, err := getShootClientWithAdmin(ctx, srscClient, s.shoot)
	if err != nil {
		updateCRBApplyFailed(&s.instance)
		return updateStatusAndStopWithError(err)
	}
	// list existing cluster role bindings
	var crbList rbacv1.ClusterRoleBindingList
	if err := shootAdminClient.List(ctx, &crbList); err != nil {
		updateCRBApplyFailed(&s.instance)
		return updateStatusAndStopWithError(err)
	}

	removed := getRemoved(crbList.Items, s.instance.Spec.Security.Administrators)
	var missing []rbacv1.ClusterRoleBinding

	// FIXME add status check
	if len(removed) == 0 && len(missing) == 0 {
		stop()
	}

	for _, fn := range []func() error{
		newDelCRBs(ctx, shootAdminClient, removed),
		newAddCRBs(ctx, shootAdminClient, missing),
	} {
		if err := fn(); err != nil {
			updateCRBApplyFailed(&s.instance)
			return updateStatusAndStopWithError(err)
		}
	}

	return updateStatusAndRequeue()
}
