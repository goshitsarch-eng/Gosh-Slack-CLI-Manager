// Package msg defines the messages components and background tasks send to
// the application, plus the action types those messages carry.
package msg

import (
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/prefs"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/slackware"
)

// Message is any application message.
type Message interface{ isMessage() }

// TaskTarget routes progress output to the component that owns a task.
type TaskTarget uint8

const (
	TargetUpdater TaskTarget = iota
	TargetSbotools
	TargetMirror
	TargetPackages
	TargetServices
	TargetNetwork
	TargetKernel
	TargetBackup
	TargetDisks
)

// Result is a success message or an error message.
type Result struct {
	Value string
	Err   string
	Ok    bool
}

// Ok builds a successful result.
func Ok(value string) Result { return Result{Value: value, Ok: true} }

// Err builds a failed result.
func Err(err string) Result { return Result{Err: err} }

// ServiceActionKind enumerates service operations.
type ServiceActionKind uint8

const (
	ServiceStart ServiceActionKind = iota
	ServiceStop
	ServiceRestart
	ServiceToggle
)

// ServiceAction is an operation on an rc.d service script.
type ServiceAction struct {
	Kind ServiceActionKind
	Name string
}

// KernelActionKind enumerates kernel operations.
type KernelActionKind uint8

const (
	KernelSetDefault KernelActionKind = iota
	KernelRunLilo
)

// KernelAction is a bootloader operation. Version is set for SetDefault.
type KernelAction struct {
	Kind    KernelActionKind
	Version string
}

// BackupActionKind enumerates backup operations.
type BackupActionKind uint8

const (
	BackupCreate BackupActionKind = iota
	BackupRestore
	BackupDelete
)

// BackupAction is a backup operation. Path is set for Restore and Delete.
type BackupAction struct {
	Kind BackupActionKind
	Path string
}

// DiskActionKind enumerates disk operations.
type DiskActionKind uint8

const (
	DiskMount DiskActionKind = iota
	DiskUnmount
	DiskCheckFilesystem
)

// DiskAction is a disk operation. Target is the device for Mount and
// CheckFilesystem, and the mount point for Unmount.
type DiskAction struct {
	Kind   DiskActionKind
	Target string
}

type (
	Quit               struct{}
	StartUpdate        struct{}
	ContinueUpdate     struct{}
	UpdateStepFinished struct {
		Step   int
		Result slackware.CommandResult
	}
	StartSbotoolsInstall struct{}
	SbotoolsStepFinished struct{ Result slackware.CommandResult }
	CreateUser           struct{}
	UserCreated          struct{ Result Result }
	SetMirror            struct{ URL string }
	// MirrorSet carries an empty Value on success.
	MirrorSet      struct{ Result Result }
	SearchPackages struct{ Query string }
	SearchResults  struct {
		Packages []slackware.PackageInfo
		Err      string
		Ok       bool
	}
	InstallPackage         struct{ Name string }
	PackageInstalled       struct{ Result Result }
	RemoveInstalledPackage struct{ Name string }
	PackageRemoved         struct{ Result Result }
	SaveConfig             struct{ Path, Content string }
	ConfigSaved            struct{ Result Result }
	SaveSettings           struct{ Settings prefs.AppSettings }
	SettingsSaved          struct{ Result Result }
	RestartNetwork         struct{}
	NetworkRestarted       struct{ Result Result }
	ServiceActionMsg       struct{ Action ServiceAction }
	ServiceActionComplete  struct{ Result Result }
	KernelActionMsg        struct{ Action KernelAction }
	KernelActionComplete   struct{ Result Result }
	BackupActionMsg        struct{ Action BackupAction }
	BackupActionComplete   struct{ Result Result }
	DiskActionMsg          struct{ Action DiskAction }
	DiskActionComplete     struct{ Result Result }
	TaskProgress           struct {
		Target TaskTarget
		Line   string
	}
)

func (Quit) isMessage()                   {}
func (StartUpdate) isMessage()            {}
func (ContinueUpdate) isMessage()         {}
func (UpdateStepFinished) isMessage()     {}
func (StartSbotoolsInstall) isMessage()   {}
func (SbotoolsStepFinished) isMessage()   {}
func (CreateUser) isMessage()             {}
func (UserCreated) isMessage()            {}
func (SetMirror) isMessage()              {}
func (MirrorSet) isMessage()              {}
func (SearchPackages) isMessage()         {}
func (SearchResults) isMessage()          {}
func (InstallPackage) isMessage()         {}
func (PackageInstalled) isMessage()       {}
func (RemoveInstalledPackage) isMessage() {}
func (PackageRemoved) isMessage()         {}
func (SaveConfig) isMessage()             {}
func (ConfigSaved) isMessage()            {}
func (SaveSettings) isMessage()           {}
func (SettingsSaved) isMessage()          {}
func (RestartNetwork) isMessage()         {}
func (NetworkRestarted) isMessage()       {}
func (ServiceActionMsg) isMessage()       {}
func (ServiceActionComplete) isMessage()  {}
func (KernelActionMsg) isMessage()        {}
func (KernelActionComplete) isMessage()   {}
func (BackupActionMsg) isMessage()        {}
func (BackupActionComplete) isMessage()   {}
func (DiskActionMsg) isMessage()          {}
func (DiskActionComplete) isMessage()     {}
func (TaskProgress) isMessage()           {}

// KeyHelp is a key hint shown in the status bar.
type KeyHelp struct {
	Key, Desc string
}
