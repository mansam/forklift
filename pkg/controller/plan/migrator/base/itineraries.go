package base

import libitr "github.com/konveyor/forklift-controller/pkg/lib/itinerary"

// Predicates.
var (
	HasPreHook              libitr.Flag = 0x01
	HasPostHook             libitr.Flag = 0x02
	RequiresConversion      libitr.Flag = 0x04
	CDIDiskCopy             libitr.Flag = 0x08
	VirtV2vDiskCopy         libitr.Flag = 0x10
	OpenstackImageMigration libitr.Flag = 0x20
	VSphere                 libitr.Flag = 0x40
)

// Phases.
const (
	Started                           = "Started"
	PreHook                           = "PreHook"
	StorePowerState                   = "StorePowerState"
	PowerOffSource                    = "PowerOffSource"
	WaitForPowerOff                   = "WaitForPowerOff"
	CreateDataVolumes                 = "CreateDataVolumes"
	WaitForDataVolumesStatus          = "WaitForDataVolumesStatus"
	WaitForFinalDataVolumesStatus     = "WaitForFinalDataVolumesStatus"
	CreateVM                          = "CreateVM"
	CopyDisks                         = "CopyDisks"
	AllocateDisks                     = "AllocateDisks"
	CopyingPaused                     = "CopyingPaused"
	AddCheckpoint                     = "AddCheckpoint"
	AddFinalCheckpoint                = "AddFinalCheckpoint"
	CreateSnapshot                    = "CreateSnapshot"
	CreateInitialSnapshot             = "CreateInitialSnapshot"
	CreateFinalSnapshot               = "CreateFinalSnapshot"
	Finalize                          = "Finalize"
	CreateGuestConversionPod          = "CreateGuestConversionPod"
	ConvertGuest                      = "ConvertGuest"
	CopyDisksVirtV2V                  = "CopyDisksVirtV2V"
	PostHook                          = "PostHook"
	Completed                         = "Completed"
	WaitForSnapshot                   = "WaitForSnapshot"
	WaitForInitialSnapshot            = "WaitForInitialSnapshot"
	WaitForFinalSnapshot              = "WaitForFinalSnapshot"
	ConvertOpenstackSnapshot          = "ConvertOpenstackSnapshot"
	StoreSnapshotDeltas               = "StoreSnapshotDeltas"
	StoreInitialSnapshotDeltas        = "StoreInitialSnapshotDeltas"
	RemovePreviousSnapshot            = "RemovePreviousSnapshot"
	RemovePenultimateSnapshot         = "RemovePenultimateSnapshot"
	RemoveFinalSnapshot               = "RemoveFinalSnapshot"
	WaitForFinalSnapshotRemoval       = "WaitForFinalSnapshotRemoval"
	WaitForPreviousSnapshotRemoval    = "WaitForPreviousSnapshotRemoval"
	WaitForPenultimateSnapshotRemoval = "WaitForPenultimateSnapshotRemoval"
)

// Steps.
const (
	Initialize      = "Initialize"
	Cutover         = "Cutover"
	DiskAllocation  = "DiskAllocation"
	DiskTransfer    = "DiskTransfer"
	ImageConversion = "ImageConversion"
	DiskTransferV2v = "DiskTransferV2v"
	VMCreation      = "VirtualMachineCreation"
	Unknown         = "Unknown"
)

const (
	TransferCompleted              = "Transfer completed."
	PopulatorPodPrefix             = "populate-"
	DvStatusCheckRetriesAnnotation = "dvStatusCheckRetries"
)

var (
	ColdItinerary = libitr.Itinerary{
		Name: "",
		Pipeline: libitr.Pipeline{
			{Name: Started},
			{Name: PreHook, All: HasPreHook},
			{Name: StorePowerState},
			{Name: PowerOffSource},
			{Name: WaitForPowerOff},
			{Name: CreateDataVolumes},
			{Name: CopyDisks, All: CDIDiskCopy},
			{Name: AllocateDisks, All: VirtV2vDiskCopy},
			{Name: CreateGuestConversionPod, All: RequiresConversion},
			{Name: ConvertGuest, All: RequiresConversion},
			{Name: CopyDisksVirtV2V, All: RequiresConversion},
			{Name: ConvertOpenstackSnapshot, All: OpenstackImageMigration},
			{Name: CreateVM},
			{Name: PostHook, All: HasPostHook},
			{Name: Completed},
		},
	}
	WarmItinerary = libitr.Itinerary{
		Name: "Warm",
		Pipeline: libitr.Pipeline{
			{Name: Started},
			{Name: PreHook, All: HasPreHook},
			{Name: CreateInitialSnapshot},
			{Name: WaitForInitialSnapshot},
			{Name: StoreInitialSnapshotDeltas, All: VSphere},
			{Name: CreateDataVolumes},
			// Precopy loop start
			{Name: WaitForDataVolumesStatus},
			{Name: CopyDisks},
			{Name: CopyingPaused},
			{Name: RemovePreviousSnapshot, All: VSphere},
			{Name: WaitForPreviousSnapshotRemoval, All: VSphere},
			{Name: CreateSnapshot},
			{Name: WaitForSnapshot},
			{Name: StoreSnapshotDeltas, All: VSphere},
			{Name: AddCheckpoint},
			// Precopy loop end
			{Name: StorePowerState},
			{Name: PowerOffSource},
			{Name: WaitForPowerOff},
			{Name: RemovePenultimateSnapshot, All: VSphere},
			{Name: WaitForPenultimateSnapshotRemoval, All: VSphere},
			{Name: CreateFinalSnapshot},
			{Name: WaitForFinalSnapshot},
			{Name: AddFinalCheckpoint},
			{Name: WaitForFinalDataVolumesStatus},
			{Name: Finalize},
			{Name: RemoveFinalSnapshot, All: VSphere},
			{Name: WaitForFinalSnapshotRemoval, All: VSphere},
			{Name: CreateGuestConversionPod, All: RequiresConversion},
			{Name: ConvertGuest, All: RequiresConversion},
			{Name: CreateVM},
			{Name: PostHook, All: HasPostHook},
			{Name: Completed},
		},
	}
)
