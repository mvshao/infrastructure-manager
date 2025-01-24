package fsm

import (
	"context"
	"time"

	gardener "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	imv1 "github.com/kyma-project/infrastructure-manager/api/v1"
	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	util "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("KIM sFnSelectShootProcessing", func() {
	testCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// GIVEN
	testScheme := runtime.NewScheme()
	util.Must(imv1.AddToScheme(testScheme))
	withTestSchemeAndObjects := func(objs ...client.Object) fakeFSMOpt {
		return func(fsm *fsm) error {
			return withFakedK8sClient(testScheme, objs...)(fsm)
		}
	}

	inputRtWithForceAnnotation := makeInputRuntimeWithAnnotation(map[string]string{"operator.kyma-project.io/force-patch-reconciliation": "true"})
	inputRtWithSuspendAnnotation := makeInputRuntimeWithAnnotation(map[string]string{"operator.kyma-project.io/suspend-patch-reconciliation": "true"})

	testShoot := gardener.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-shoot",
			Namespace: "garden-",
		},
		Spec: gardener.ShootSpec{
			DNS: &gardener.DNS{
				Domain: ptr.To("test-domain"),
			},
		},
		Status: gardener.ShootStatus{
			LastOperation: &gardener.LastOperation{
				State:          gardener.LastOperationStateSucceeded,
			},
		},
	}

	testFunction := buildTestFunction(sFnSelectShootProcessing)

	DescribeTable(
		"transition graph validation for sFnSelectShootProcessing",
		testFunction,
		Entry(
			"should switch to sFnPatchExistingShoot due to force reconciliation annotation",
			testCtx,
			must(newFakeFSM, withTestFinalizer, withTestSchemeAndObjects()),
			&systemState{instance: *inputRtWithForceAnnotation, shoot: &testShoot},
			testOpts{
				MatchExpectedErr: BeNil(),
				MatchNextFnState: haveName("sFnPatchExistingShoot"),
			},
		),
		Entry(
			"should stop due to suspend annotation",
			testCtx,
			must(newFakeFSM, withTestFinalizer, withTestSchemeAndObjects()),
			&systemState{instance: *inputRtWithSuspendAnnotation, shoot: &testShoot},
			testOpts{
				MatchExpectedErr: BeNil(),
				MatchNextFnState: BeNil(),
			},
		),
	)
})

func makeInputRuntimeWithAnnotation(annotations map[string]string) *imv1.Runtime {
	return &imv1.Runtime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-shoot",
			Namespace: "kcp-system",
			Labels: map[string]string{
				"kyma-project.io/instance-id":         "instance-id",
				"kyma-project.io/runtime-id":          "runtime-id",
				"kyma-project.io/shoot-name":          "shoot-name",
				"kyma-project.io/region":              "region",
				"operator.kyma-project.io/kyma-name":  "kyma-name",
				"kyma-project.io/broker-plan-id":      "broker-plan-id",
				"kyma-project.io/broker-plan-name":    "broker-plan-name",
				"kyma-project.io/global-account-id":   "global-account-id",
				"kyma-project.io/subaccount-id":       "subaccount-id",
				"operator.kyma-project.io/managed-by": "managed-by",
				"operator.kyma-project.io/internal":   "false",
				"kyma-project.io/platform-region":     "platform-region",
			},
			Annotations: annotations,
		},
		Spec: imv1.RuntimeSpec{
			Shoot: imv1.RuntimeShoot{
				Name:     "test-shoot",
				Region:   "region",
				Provider: imv1.Provider{Type: "aws"},
			},
		},
	}
}
