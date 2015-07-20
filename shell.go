package scipipe

import (
	"fmt"
	"os/exec"
	re "regexp"
	str "strings"
)

type ShellTask struct {
	_OutOnly     bool
	InPorts      map[string]chan *fileTarget
	InPaths      map[string]string
	OutPorts     map[string]chan *fileTarget
	OutPathFuncs map[string]func() string
	Command      string
}

func NewShellTask(command string, outOnly bool) *ShellTask {
	t := new(ShellTask)
	t.Command = command
	t._OutOnly = outOnly
	if !t._OutOnly {
		t.InPorts = make(map[string]chan *fileTarget)
		t.InPaths = make(map[string]string)
	}
	t.OutPorts = make(map[string]chan *fileTarget)
	t.OutPathFuncs = make(map[string]func() string)
	return t
}

func Sh(cmd string) *ShellTask {

	// Determine whether there are any inport, or if this task is "out only"
	outOnly := false
	r, err := re.Compile(".*{i:([^{}:]+)}.*")
	Check(err)
	if !r.MatchString(cmd) {
		outOnly = true
	}

	// Create task
	t := NewShellTask(cmd, outOnly)

	if t._OutOnly {
		// Find out port names, and set up in port lists
		r, err := re.Compile("{o:([^{}:]+)}")
		Check(err)
		ms := r.FindAllStringSubmatch(cmd, -1)
		for _, m := range ms {
			name := m[1]
			t.OutPorts[name] = make(chan *fileTarget, BUFSIZE)
		}
	} else {
		// Find in/out port names, and set up in port lists
		r, err := re.Compile("{(o|i):([^{}:]+)}")
		Check(err)
		ms := r.FindAllStringSubmatch(cmd, -1)
		for _, m := range ms {
			typ := m[1]
			name := m[2]
			if typ == "o" {
				t.OutPorts[name] = make(chan *fileTarget, BUFSIZE)
			} else if typ == "i" {
				// Set up a channel on the inports, even though this is
				// often replaced by another tasks output port channel.
				// It might be nice to have it init'ed with a channel
				// anyways, for use cases when we want to send fileTargets
				// on the inport manually.
				t.InPorts[name] = make(chan *fileTarget, BUFSIZE)
			}
		}
	}

	return t
}

func (t *ShellTask) Init() {
	go func() {
		if t._OutOnly {

			t.executeCommands(t.Command)

			// Send output targets
			for oname, ochan := range t.OutPorts {
				fn := t.OutPathFuncs[oname]
				baseName := fn()
				nf := NewFileTarget(baseName)
				ochan <- nf
				close(ochan)
			}
		} else {
			for {
				doClose := false
				// Set up inport / path mappings
				for iname, ichan := range t.InPorts {
					infile, open := <-ichan
					if !open {
						doClose = true
					} else {
						t.InPaths[iname] = infile.GetPath()
					}
				}

				if !doClose {
					t.executeCommands(t.Command)
					// Send output targets
					for oname, ochan := range t.OutPorts {
						fn := t.OutPathFuncs[oname]
						baseName := fn()
						nf := NewFileTarget(baseName)
						ochan <- nf
					}
				} else {
					// Close output channels
					for _, ochan := range t.OutPorts {
						close(ochan)
					}
					// Break out of the main loop
					break
				}

			}
		}
	}()
}

func (t *ShellTask) executeCommands(cmd string) {
	cmd = t.ReplacePortDefsInCmd(cmd)
	fmt.Println("ShellTask: Executing command: ", cmd)
	_, err := exec.Command("bash", "-c", cmd).Output()
	Check(err)
}

func (t *ShellTask) ReplacePortDefsInCmd(cmd string) string {
	r, err := re.Compile("{(o|i):([^{}:]+)}")
	Check(err)
	ms := r.FindAllStringSubmatch(cmd, -1)
	for _, m := range ms {
		whole := m[0]
		typ := m[1]
		name := m[2]
		newstr := "REPLACE_FAILED_FOR_PORT_" + name + "_CHECK_YOUR_CODE"
		if typ == "o" {
			newstr = t.OutPathFuncs[name]()
		} else if typ == "i" {
			newstr = t.InPaths[name]
		}
		cmd = str.Replace(cmd, whole, newstr, -1)
	}
	return cmd
}

func (t *ShellTask) GetInPath(inPort string) string {
	inPath := t.InPaths[inPort]
	return inPath
}