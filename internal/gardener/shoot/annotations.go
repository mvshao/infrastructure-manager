package shoot

import (
	gardener "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	imv1 "github.com/kyma-project/infrastructure-manager/api/v1"
)

func annotationsExtender(imv1.RuntimeShoot, *gardener.Shoot) error {
	return nil
}
