package ocp

import (
	"errors"
	"fmt"

	planapi "github.com/konveyor/forklift-controller/pkg/apis/forklift/v1beta1/plan"
	"github.com/konveyor/forklift-controller/pkg/controller/plan"
	plancontext "github.com/konveyor/forklift-controller/pkg/controller/plan/context"
	"github.com/konveyor/forklift-controller/pkg/controller/plan/migrator/base"
	"github.com/konveyor/forklift-controller/pkg/controller/provider/web"
	libitr "github.com/konveyor/forklift-controller/pkg/lib/itinerary"
	"github.com/konveyor/forklift-controller/pkg/lib/logging"
	cdi "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

const (
	Warm      = "warm"
	Cold      = "cold"
	Live      = "live"
	Migration = "migration"
	Plan      = "plan"
	VM        = "vm"
)

// Phases
const (
	Started                 = "Started"
	PreHook                 = "PreHook"
	EnsureResources         = "EnsureResources"
	SynchronizeCertificates = "SynchronizeCertificates"
	CreateEmptyDataVolumes  = "CreateEmptyDataVolumes"
	CreateTargetVM          = "CreateTargetVM"
	CreateTargetMigration   = "CreateTargetMigration"
	WaitForTargetMigration  = "WaitForTargetMigration"
	CreateSourceMigration   = "CreateSourceMigration"
	WaitForStateTransfer    = "WaitForStateTransfer"
	PostHook                = "PostHook"
	Completed               = "Completed"
)

// Pipeline
const (
	PrepareTarget   = "PrepareTarget"
	Synchronization = "Synchronization"
)

// Conditions
const (
	Running = "Running"
)

// Package logger.
var log = logging.WithName("migrator|ocp")

func New(context *plancontext.Context, kubevirt plan.KubeVirt) (migrator base.Migrator, err error) {
	switch context.Plan.Spec.Type {
	case Live:
		m := LiveMigrator{Context: context, kubevirt: kubevirt}
		err = m.Init()
		if err != nil {
			return
		}
		migrator = &m
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

type LiveMigrator struct {
	Context  *plancontext.Context
	kubevirt plan.KubeVirt
}

func (r *LiveMigrator) Init() error {
	return nil
}

func (r *LiveMigrator) Status(vm planapi.VM) (status *planapi.VMStatus) {
	if current, found := r.Context.Plan.Status.Migration.FindVM(vm.Ref); !found {
		status = &planapi.VMStatus{VM: vm}
		if r.Context.Plan.Spec.Warm {
			status.Warm = &planapi.Warm{}
		}
	} else {
		status = current
	}
	return
}

func (r *LiveMigrator) Reset(status *planapi.VMStatus, pipeline []*planapi.Step) {
	status.DeleteCondition(base.Canceled, base.Failed)
	status.MarkReset()
	itr := r.Itinerary()
	step, _ := itr.First()
	status.Phase = step.Name
	status.Pipeline = pipeline
	status.Error = nil
	if r.Context.Plan.Spec.Warm {
		status.Warm = &planapi.Warm{}
	}
	return
}

func (r *LiveMigrator) Pipeline(vm planapi.VM) (pipeline []*planapi.Step, err error) {
	itinerary := r.Itinerary()
	step, _ := itinerary.First()
	for {
		switch step.Name {
		case Started:
			pipeline = append(
				pipeline,
				&planapi.Step{
					Task: planapi.Task{
						Name:        base.Initialize,
						Description: "Initialize migration.",
						Progress:    libitr.Progress{Total: 1},
						Phase:       base.Pending,
					},
				})
		case PreHook:
			pipeline = append(
				pipeline,
				&planapi.Step{
					Task: planapi.Task{
						Name:        PreHook,
						Description: "Run pre-migration hook.",
						Progress:    libitr.Progress{Total: 1},
						Phase:       base.Pending,
					},
				})
		case PostHook:
			pipeline = append(
				pipeline,
				&planapi.Step{
					Task: planapi.Task{
						Name:        PostHook,
						Description: "Run post-migration hook.",
						Progress:    libitr.Progress{Total: 1},
						Phase:       base.Pending,
					},
				})
		case EnsureResources, CreateEmptyDataVolumes, CreateTargetVM:
			pipeline = append(
				pipeline,
				&planapi.Step{
					Task: planapi.Task{
						Name:        PrepareTarget,
						Description: "Prepare target namespace.",
						Progress:    libitr.Progress{Total: 1},
						Phase:       base.Pending,
					},
				})
		case CreateTargetMigration, WaitForTargetMigration, CreateSourceMigration, WaitForStateTransfer:
			pipeline = append(
				pipeline,
				&planapi.Step{
					Task: planapi.Task{
						Name:        Synchronization,
						Description: "Synchronize source and target VMs.",
						Progress:    libitr.Progress{Total: 1},
						Phase:       base.Pending,
					},
				})
		}
		next, done, _ := itinerary.Next(step.Name)
		if !done {
			step = next
		} else {
			break
		}
	}

	log.V(2).Info(
		"Pipeline built.",
		"vm",
		vm.String())
	return
}

func (r *LiveMigrator) ExecutePhase(vm *planapi.VMStatus) (ok bool, err error) {
	step, found := vm.FindStep(r.Step(vm))
	if !found {
		vm.AddError(fmt.Sprintf("Step '%s' not found", r.Step(vm)))
		ok = true
		return
	}
	switch vm.Phase {
	case Started:
		vm.MarkedStarted()
		step.Phase = Completed
	case PreHook, PostHook:
		// delegate to common pipeline
		return
	case EnsureResources:
		step.MarkStarted()
		step.Phase = Running
	case SynchronizeCertificates:
	case CreateEmptyDataVolumes:
		var dataVolumes []cdi.DataVolume
		dataVolumes, err = r.kubevirt.DataVolumes(vm)
		if err != nil {
			if !errors.As(err, &web.ProviderNotReadyError{}) {
				log.Error(err, "error creating volumes", "vm", vm.Name)
				step.AddError(err.Error())
				err = nil
			}
			return
		}
		err = r.kubevirt.EnsureDataVolumes(vm, dataVolumes)
		if err != nil {
			if !errors.As(err, &web.ProviderNotReadyError{}) {
				step.AddError(err.Error())
				err = nil
			}
			return
		}
	case CreateTargetVM:
		err = r.kubevirt.EnsureVM(vm)
		if err != nil {
			if !errors.As(err, &web.ProviderNotReadyError{}) {
				step.AddError(err.Error())
				err = nil
			}
			return
		}
		step.MarkCompleted()
		step.Phase = Completed
	case CreateTargetMigration:
		step.MarkStarted()
		step.Phase = Running
	case WaitForTargetMigration:
	case CreateSourceMigration:
	case WaitForStateTransfer:
		step.MarkCompleted()
		step.Phase = Completed
	default:
		log.Info(
			"Phase unknown.",
			"vm",
			vm)
		vm.AddError(
			fmt.Sprintf(
				"Phase [%s] unknown",
				vm.Phase))
		vm.Phase = Completed
		return
	}
	vm.Phase = r.Next(vm)
	ok = true
	return
}

func (r *LiveMigrator) Itinerary() (itinerary *libitr.Itinerary) {
	itinerary = &libitr.Itinerary{
		Name: "ocp-live",
		Pipeline: libitr.Pipeline{
			{Name: Started},
			{Name: PreHook},
			{Name: EnsureResources},
			{Name: SynchronizeCertificates},
			{Name: CreateEmptyDataVolumes},
			{Name: CreateTargetVM},
			{Name: CreateTargetMigration},
			{Name: WaitForTargetMigration},
			{Name: CreateSourceMigration},
			{Name: WaitForStateTransfer},
			{Name: PostHook},
			{Name: Completed},
		},
	}
	return
}

func (r *LiveMigrator) Next(status *planapi.VMStatus) (next string) {
	itinerary := r.Itinerary()
	step, done, err := itinerary.Next(status.Phase)
	if done || err != nil {
		next = Completed
		if err != nil {
			log.Error(err, "Next phase failed.")
		}
	} else {
		next = step.Name
	}
	log.Info("Itinerary transition", "current phase", status.Phase, "next phase", next)
	return
}

func (r *LiveMigrator) Step(status *planapi.VMStatus) (step string) {
	switch status.Phase {
	case Started:
		step = base.Initialize
	case PreHook, PostHook:
		step = status.Phase
	case EnsureResources, CreateEmptyDataVolumes, CreateTargetVM:
		step = PrepareTarget
	case CreateTargetMigration, WaitForTargetMigration, CreateSourceMigration, WaitForStateTransfer:
		step = Synchronization
	default:
		step = base.Unknown
	}
	return
}

//
//type LiveBuilder struct {
//	*plancontext.Context
//	sourceClient client.Client
//}
//
//func (r *LiveBuilder) vmLabels(vmRef ref.Ref) map[string]string {
//	labels := r.planLabels()
//	labels[VM] = vmRef.ID
//	return labels
//}
//
//func (r *LiveBuilder) planLabels() map[string]string {
//	return map[string]string{
//		Migration: string(r.Migration.UID),
//		Plan:      string(r.Plan.GetUID()),
//	}
//}
