package cleaner

import (
	"context"
	imv1 "github.com/kyma-project/infrastructure-manager/api/v1"
	"log/slog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const deleteAfter = 1 * time.Second

type RuntimeCleaner struct {
	k8sClient client.Client
	log       *slog.Logger
}

func NewRuntimeCleaner(k8sClient client.Client, log *slog.Logger) *RuntimeCleaner {
	return &RuntimeCleaner{k8sClient: k8sClient, log: log}
}

func (r RuntimeCleaner) Execute() error {

	err := r.removeOldRuntimes()
	if err != nil {
		r.log.With("error", err).Error("Error during removing old runtimes ")
		return err
	}
	return nil
}

func (r RuntimeCleaner) removeOldRuntimes() error {
	runtimes := &imv1.RuntimeList{}
	if err := r.k8sClient.List(context.Background(), runtimes); err != nil {
		return err
	}

	for _, runtimeObj := range runtimes.Items {
		if isTimeForCleanup(runtimeObj) && isControlledByKIM(runtimeObj) && runtimeObj.DeletionTimestamp == nil {
			err := r.k8sClient.Delete(context.Background(), &runtimeObj)
			if err != nil {
				return err
			}
			r.log.With("runtime", runtimeObj.Name).Info("Runtime was marked to be removed by KIM reconciler")
		}
	}
	return nil
}

func isTimeForCleanup(runtimeObj imv1.Runtime) bool {
	return runtimeObj.CreationTimestamp.Add(deleteAfter).Before(time.Now())
}

func isControlledByKIM(runtimeObj imv1.Runtime) bool {
	return runtimeObj.Labels["kyma-project.io/controlled-by-provisioner"] == "false"
}