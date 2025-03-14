package migrator

import (
	"github.com/konveyor/forklift-controller/pkg/apis/forklift/v1beta1"
	plancontext "github.com/konveyor/forklift-controller/pkg/controller/plan/context"
	"github.com/konveyor/forklift-controller/pkg/controller/plan/migrator/base"
	"github.com/konveyor/forklift-controller/pkg/controller/plan/migrator/ocp"
)

type Migrator = base.Migrator

func New(context *plancontext.Context) (migrator Migrator, err error) {
	switch context.Source.Provider.Type() {
	case v1beta1.OpenShift:
		migrator, err = ocp.New(context)
		return
	default:
		m := base.BaseMigrator{Context: context}
		err = m.Init()
		if err != nil {
			return
		}
		migrator = &m
	}
	return
}
