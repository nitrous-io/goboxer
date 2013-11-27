// This contains definition of `Message` type that Boxer and Coach uses to
// communicate with each other
package goboxer

// we concat all messages type here and use `Receive` to distribute correct
// type to correct function for now
type Message struct {
	// all messages can have these
	Id         string            `json:"id,omitempty"`
	BoxId      int               `json:"boxId,omitempty,string"`
	Type       string            `json:"type,omitempty"`
	ReturnVars map[string]string `json:"returnVars,omitempty"`

	// for advertise message
	FreeMemory int      `json:"freeMemory,omitempty"`
	Runtimes   []string `json:"runtimes,omitempty"`

	// for create message
	// Name is used subsequently in coach message to describe containerName
	// i.e. 123-testbox
	Name          string   `json:"name,omitempty"`
	Port          int      `json:"port,omitempty"`
	Runtime       string   `json:"runtime,omitempty"`
	SnapshotURL   string   `json:"snapshotURL,omitempty"`
	Memory        string   `json:"memory,omitempty"`
	DiskSpace     string   `json:"diskSpace,omitempty"`
	PublicKeys    []string `json:"publicKeys,omitempty"`
	SshKeyPair    []string `json:"sshKeyPair,omitempty"`
	CpuShares     int      `json:"cpuShares,omitempty"`
	VirtualMemory string   `json:"virtualMemory,omitempty"`

	// for normal message
	DataType string      `json:"dataType,omitempty"`
	Data     interface{} `json:"data,omitempty"`
	Success  bool        `json:"success,omitempty"`

	// For info message
	Info BoxInfo `json:"info,omitempty"`
}

type MessageChannel chan Message
