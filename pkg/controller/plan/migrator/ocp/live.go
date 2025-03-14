package ocp

import (
	"fmt"

	"github.com/konveyor/forklift-controller/pkg/apis/forklift/v1beta1"
	"github.com/konveyor/forklift-controller/pkg/apis/forklift/v1beta1/plan"
	"github.com/konveyor/forklift-controller/pkg/apis/forklift/v1beta1/ref"
	plancontext "github.com/konveyor/forklift-controller/pkg/controller/plan/context"
	"github.com/konveyor/forklift-controller/pkg/controller/plan/migrator/base"
	model "github.com/konveyor/forklift-controller/pkg/controller/provider/model/ocp"
	liberr "github.com/konveyor/forklift-controller/pkg/lib/error"
	libitr "github.com/konveyor/forklift-controller/pkg/lib/itinerary"
	"github.com/konveyor/forklift-controller/pkg/lib/logging"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	cnv "kubevirt.io/api/core/v1"
	cdi "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func New(context *plancontext.Context) (migrator base.Migrator, err error) {
	switch context.Plan.Spec.Type {
	case Live:
		m := LiveMigrator{Context: context}
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
	Context *plancontext.Context
}

func (r *LiveMigrator) Init() error {
	return nil
}

func (r *LiveMigrator) Status(vm plan.VM) (status *plan.VMStatus) {
	if current, found := r.Context.Plan.Status.Migration.FindVM(vm.Ref); !found {
		status = &plan.VMStatus{VM: vm}
		if r.Context.Plan.Spec.Warm {
			status.Warm = &plan.Warm{}
		}
	} else {
		status = current
	}
	return
}

func (r *LiveMigrator) Reset(status *plan.VMStatus, pipeline []*plan.Step) {
	status.DeleteCondition(base.Canceled, base.Failed)
	status.MarkReset()
	itr := r.Itinerary()
	step, _ := itr.First()
	status.Phase = step.Name
	status.Pipeline = pipeline
	status.Error = nil
	if r.Context.Plan.Spec.Warm {
		status.Warm = &plan.Warm{}
	}
	return
}

func (r *LiveMigrator) Pipeline(vm plan.VM) (pipeline []*plan.Step, err error) {
	itinerary := r.Itinerary()
	step, _ := itinerary.First()
	for {
		switch step.Name {
		case Started:
			pipeline = append(
				pipeline,
				&plan.Step{
					Task: plan.Task{
						Name:        base.Initialize,
						Description: "Initialize migration.",
						Progress:    libitr.Progress{Total: 1},
						Phase:       base.Pending,
					},
				})
		case PreHook:
			pipeline = append(
				pipeline,
				&plan.Step{
					Task: plan.Task{
						Name:        PreHook,
						Description: "Run pre-migration hook.",
						Progress:    libitr.Progress{Total: 1},
						Phase:       base.Pending,
					},
				})
		case PostHook:
			pipeline = append(
				pipeline,
				&plan.Step{
					Task: plan.Task{
						Name:        PostHook,
						Description: "Run post-migration hook.",
						Progress:    libitr.Progress{Total: 1},
						Phase:       base.Pending,
					},
				})
		case EnsureResources, CreateEmptyDataVolumes, CreateTargetVM:
			pipeline = append(
				pipeline,
				&plan.Step{
					Task: plan.Task{
						Name:        PrepareTarget,
						Description: "Prepare target namespace.",
						Progress:    libitr.Progress{Total: 1},
						Phase:       base.Pending,
					},
				})
		case CreateTargetMigration, WaitForTargetMigration, CreateSourceMigration, WaitForStateTransfer:
			pipeline = append(
				pipeline,
				&plan.Step{
					Task: plan.Task{
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

func (r *LiveMigrator) ExecutePhase(vm *plan.VMStatus) (ok bool, err error) {
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
	case SynchronizeCertificates:
	case CreateEmptyDataVolumes:
	case CreateTargetVM:
		step.MarkStarted()
		step.Phase = Running
		//err = r.EnsureVM(vm.Ref)
		//if err != nil {
		//}
	case CreateTargetMigration:
	case WaitForTargetMigration:
	case CreateSourceMigration:
	case WaitForStateTransfer:
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

func (r *LiveMigrator) Next(status *plan.VMStatus) (next string) {
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

func (r *LiveMigrator) Step(status *plan.VMStatus) (step string) {
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

type LiveBuilder struct {
	*plancontext.Context
	sourceClient client.Client
}

func (r *LiveBuilder) CreateBlankDataVolumes(vmRef ref.Ref) (err error) {
	vm := &model.VM{}
	err = r.Source.Inventory.Find(vm, vmRef)
	if err != nil {
		err = liberr.Wrap(err, "vm", vmRef.String())
		return
	}

	storageMap := map[string]v1beta1.DestinationStorage{}
	for _, storage := range r.Map.Storage.Spec.Map {
		storageMap[storage.Source.Name] = storage.Destination
	}

	for _, vol := range vm.Object.Spec.Template.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			volRef := ref.Ref{
				Name:      vol.PersistentVolumeClaim.ClaimName,
				Namespace: vm.Namespace,
			}
			pvc := &model.PersistentVolumeClaim{}
			err = r.Source.Inventory.Find(pvc, volRef)
			if err != nil {
				err = liberr.Wrap(err, "vm", vmRef.String(), "pvc", volRef.String())
				return
			}
			size, sErr := r.size(pvc)
			if sErr != nil {
				err = liberr.Wrap(sErr, "vm", vmRef.String(), "pvc", volRef.String())
				return
			}
			mapping := storageMap[*pvc.Object.Spec.StorageClassName]
			dv := &cdi.DataVolume{Spec: cdi.DataVolumeSpec{
				Source: &cdi.DataVolumeSource{
					Blank: &cdi.DataVolumeBlankImage{},
				},
				Storage: &cdi.StorageSpec{
					AccessModes: nil,
					Resources: core.ResourceRequirements{
						Requests: core.ResourceList{
							core.ResourceStorage: size,
						},
					},
				},
			}}
			if mapping.AccessMode != "" {
				dv.Spec.Storage.AccessModes = []core.PersistentVolumeAccessMode{mapping.AccessMode}
			}
			if mapping.VolumeMode != "" {
				dv.Spec.Storage.VolumeMode = &mapping.VolumeMode
			}
		}
	}
	//vm.Object.Spec.Template.Spec.Volumes[0].
	return
}

func (r *LiveBuilder) size(pvc *model.PersistentVolumeClaim) (size resource.Quantity, err error) {
	storageRequest := pvc.Object.Spec.Resources.Requests.Storage()
	if storageRequest == nil {
		storageRequest = pvc.Object.Status.Capacity.Storage()
	}
	if storageRequest == nil {
		err = liberr.New("Unable to determine resource requirements for PVC.")
		return
	}
	size = *storageRequest
	return
	//
}

func (r *LiveBuilder) EnsureVM(vmRef ref.Ref) (err error) {
	source := &model.VM{}
	err = r.Source.Inventory.Find(source, vmRef)
	if err != nil {
		err = liberr.Wrap(err, "vm", vmRef.String())
		return
	}

	destination := cnv.VirtualMachine{
		TypeMeta: source.Object.TypeMeta,
		ObjectMeta: meta.ObjectMeta{
			Labels:      source.Object.ObjectMeta.Labels,
			Annotations: source.Object.ObjectMeta.Annotations,
			Name:        source.Object.ObjectMeta.Name,
			Namespace:   r.Plan.Spec.TargetNamespace,
		},
		Spec: source.Object.Spec,
	}
	destination.Spec.Running = nil
	destination.Spec.RunStrategy = nil

	//err = r.Destination.Client.Create()
	return
}

func (r *LiveBuilder) vmLabels(vmRef ref.Ref) map[string]string {
	labels := r.planLabels()
	labels[VM] = vmRef.ID
	return labels
}

func (r *LiveBuilder) planLabels() map[string]string {
	return map[string]string{
		Migration: string(r.Migration.UID),
		Plan:      string(r.Plan.GetUID()),
	}
}
