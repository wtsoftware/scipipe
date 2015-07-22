package scipipe

import (
	"fmt"
	"os/exec"
	re "regexp"
	str "strings"
	// "time"
	"errors"
)

type ShellTask struct {
	task
	_OutOnly     bool
	InPorts      map[string]chan *FileTarget
	InPaths      map[string]string
	OutPorts     map[string]chan *FileTarget
	OutPathFuncs map[string]func() string
	Params       map[string]chan string
	Command      string
}

func NewShellTask(command string) *ShellTask {
	return &ShellTask{
		Command:      command,
		InPorts:      make(map[string]chan *FileTarget),
		InPaths:      make(map[string]string),
		OutPorts:     make(map[string]chan *FileTarget),
		OutPathFuncs: make(map[string]func() string),
		Params:       make(map[string]chan string),
	}
}

func Sh(cmd string) *ShellTask {
	t := NewShellTask(cmd)
	t.initPortsFromCmdPattern(cmd)
	return t
}

func ShParams(cmd string, params map[string]string) *ShellTask {
	t := NewShellTask(cmd)
	t.initPortsFromCmdPattern(cmd)
	if params != nil {
		// Send eternal list of options
		go func() {
			for name, val := range params {
				t.Params[name] <- val
			}
		}()
	}
	return t
}

func (t *ShellTask) initPortsFromCmdPattern(cmd string) {
	// Find in/out port names and Params and set up in struct fields
	r := getPlaceHolderRegex()
	ms := r.FindAllStringSubmatch(cmd, -1)
	for _, m := range ms {
		if len(m) < 3 {
			Check(errors.New("Too few matches"))
		}
		typ := m[1]
		name := m[2]
		if typ == "o" {
			t.OutPorts[name] = make(chan *FileTarget, BUFSIZE)
		} else if typ == "p" {
			t.Params[name] = make(chan string, BUFSIZE)
		}

		// else if typ == "i" {
		// Set up a channel on the inports, even though this is
		// often replaced by another tasks output port channel.
		// It might be nice to have it init'ed with a channel
		// anyways, for use cases when we want to send FileTargets
		// on the inport manually.
		// t.InPorts[name] = make(chan *FileTarget, BUFSIZE)
		// }
	}
}

func (t *ShellTask) Run() {
	fmt.Println("Entering task: ", t.Command)
	defer t.closeOutChans()

	// Main loop
	continueLoop := true
	for continueLoop {
		// If there are no inports, we know we should exit the loop
		// directly after executing the command, and sending the outputs
		if len(t.InPorts) == 0 {
			continueLoop = false
		}

		// Read from inports
		inPortsOpen := t.receiveInputs()
		if !inPortsOpen {
			break
		}

		// Execute command
		t.formatAndExecute(t.Command)

		// Send
		t.sendOutputs()
	}
	fmt.Println("Exiting task:  ", t.Command)
}

func (t *ShellTask) closeOutChans() {
	// Close output channels
	for _, ochan := range t.OutPorts {
		close(ochan)
	}
}

func (t *ShellTask) receiveInputs() bool {
	inPortsOpen := true
	// Read input targets on in-ports and set up path mappings
	for iname, ichan := range t.InPorts {
		infile, open := <-ichan
		if !open {
			inPortsOpen = false
			continue
		}
		fmt.Println("Receiving file:", infile.GetPath())
		t.InPaths[iname] = infile.GetPath()
	}
	return inPortsOpen
}

func (t *ShellTask) sendOutputs() {
	// Send output targets on out ports
	for oname, ochan := range t.OutPorts {
		fun := t.OutPathFuncs[oname]
		baseName := fun()
		ft := NewFileTarget(baseName)
		fmt.Println("Sending file:  ", ft.GetPath())
		ochan <- ft
	}
}

func (t *ShellTask) formatAndExecute(cmd string) {
	cmd = t.replacePlaceholdersInCmd(cmd)
	fmt.Println("Executing cmd: ", cmd)
	_, err := exec.Command("bash", "-c", cmd).Output()
	Check(err)
}

func (t *ShellTask) replacePlaceholdersInCmd(cmd string) string {
	r := getPlaceHolderRegex()
	ms := r.FindAllStringSubmatch(cmd, -1)
	for _, m := range ms {
		whole := m[0]
		typ := m[1]
		name := m[2]
		var newstr string
		if typ == "o" {
			if t.OutPathFuncs[name] == nil {
				msg := fmt.Sprint("Missing outpath function for outport '", name, "' of shell task '", t.Command, "'")
				Check(errors.New(msg))
			} else {
				newstr = t.OutPathFuncs[name]()
			}
		} else if typ == "i" {
			if t.InPaths[name] == "" {
				msg := fmt.Sprint("Missing inpath for inport '", name, "' of shell task '", t.Command, "'")
				Check(errors.New(msg))
			} else {
				newstr = t.InPaths[name]
			}
		}
		if newstr == "" {
			msg := fmt.Sprint("Replace failed for port ", name, " in task '", t.Command, "'")
			Check(errors.New(msg))
		}
		cmd = str.Replace(cmd, whole, newstr, -1)
	}
	return cmd
}

func (t *ShellTask) GetInPath(inPort string) string {
	var inPath string
	if t.InPaths[inPort] != "" {
		inPath = t.InPaths[inPort]
	} else {
		msg := fmt.Sprint("t.GetInPath(): Missing inpath for inport '", inPort, "' of shell task '", t.Command, "'")
		Check(errors.New(msg))
	}
	return inPath
}

func getPlaceHolderRegex() *re.Regexp {
	r, err := re.Compile("{(o|i|p):([^{}:]+)}")
	Check(err)
	return r
}
