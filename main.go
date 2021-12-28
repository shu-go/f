package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shu-go/gli"
)

type Command struct {
	Name string
	Path string
	Args []string
}

type Config struct {
	Commands []Command
}

type globalCmd struct {
	Add    bool `help:"add/replace a command"`
	Remove bool `help:"remove a command"`

	List     bool `cli:"list,list-by-name"`
	ListPath bool `cli:"list-by-path"`
}

const userConfigFolder = "faker"

// Version is app version
var Version string

func init() {
	if Version == "" {
		Version = "dev-" + time.Now().Format("20060102")
	}
}

// not work yet
func (c globalCmd) Before(args []string) error {
	if c.Add && c.Remove {
		return errors.New("don't pass both --add and --remove!!")
	}

	if c.Add && len(args) < 2 {
		return errors.New("pass both Name and Path after --add")
	}

	if c.Remove && len(args) < 1 {
		return errors.New("pass Name after --remove")
	}

	return nil
}

func main() {
	appname, err := os.Executable()
	if err != nil {
		appname = "f"
	} else {
		appname = filepath.Base(appname)
		ext := filepath.Ext(appname)
		appname = appname[:len(appname)-len(ext)]
	}

	app := gli.NewWith(&globalCmd{})
	app.Name = appname
	app.Desc = "command faker"
	app.Version = Version
	app.Usage = `# add (replace) a command
` + appname + ` --add gitinit git init
` + appname + ` --add goinit go mod init
# list commands
` + appname + `
# run a command
` + appname + ` gitinit
# remove a command
` + appname + ` --remove gitinit
# make another faker
copy ` + appname + ` another.exe
another --add gitinit echo hoge hoge

----

config dir:
    1. exe path
        ` + appname + `.json
        Place the json in the same location as the executable.
    2. config directory 
        {CONFIG_DIR}/` + userConfigFolder + `/` + appname + `.json
        Windows: %appdata%\` + userConfigFolder + `\` + appname + `.json
        (see https://cs.opensource.google/go/go/+/go1.17.3:src/os/file.go;l=457)
    If none of 1,2 files exist, --add writes to 1.
`
	app.Copyright = "(C) 2021 Shuhei Kubota"
	ci, args, err := app.Parse(os.Args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if ci == nil {
		return
	}

	c := ci.(*globalCmd)

	if c.Add && c.Remove {
		fmt.Fprintln(os.Stderr, errors.New("don't pass both --add and --remove!!"))
		return
	}

	if c.Add && len(args) < 2 {
		fmt.Fprintln(os.Stderr, errors.New("pass both Name and Path after --add"))
		return
	}

	if c.Remove && len(args) < 1 {
		fmt.Fprintln(os.Stderr, errors.New("pass Name after --remove"))
		return
	}

	configPath := determineConfigPath()

	config, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	if len(args) < 1 || c.List || c.ListPath {
		if c.ListPath {
			sort.Slice(config.Commands, func(i, j int) bool {
				if config.Commands[i].Path < config.Commands[j].Path {
					return true
				} else {
					len1 := len(config.Commands[i].Args)
					len2 := len(config.Commands[j].Args)
					if len1 > len2 {
						len1 = len2
					}

					for k := 0; k < len1; k++ {
						if config.Commands[i].Args[k] < config.Commands[j].Args[k] {
							return true
						}
					}
				}
				return false
			})
		}

		fmt.Println("commands:")
		for _, c := range config.Commands {
			fmt.Printf("\t%v\t%v %v\n", c.Name, c.Path, c.Args)
		}

		fmt.Println("")
		fmt.Println("config:", configPath)

		return
	}

	if c.Add {
		n := args[0]
		p := args[1]
		a := args[2:]

		addCommand(config, n, p, a)
		err := saveConfig(configPath, config)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		return
	}

	if c.Remove {
		n := args[0]
		removeCommand(config, n)

		err := saveConfig(configPath, config)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		return
	}

	fcmd := findCommand(config, args[0])
	if fcmd == nil {
		fmt.Fprintln(os.Stderr, args[0]+" not found")
		return
	}

	oscmds := make([]exec.Cmd, 1)
	curr := &oscmds[0]
	curr.Path = fcmd.Path
	p, err := exec.LookPath(curr.Path)
	if err == nil {
		curr.Path = p
	}
	curr.Args = append(curr.Args, fcmd.Path)
	//rog.Print("fcmd.Args:", fcmd.Args)
	for i, a := range fcmd.Args {
		//rog.Print(a)

		if strings.HasPrefix(a, "|") {
			oscmds = append(oscmds, exec.Cmd{})
			curr = &oscmds[len(oscmds)-1]

			//rog.Print("new cmd")

			if fcmd.Args[i] != "|" {
				curr.Path = a[1:]
				p, err := exec.LookPath(curr.Path)
				if err == nil {
					curr.Path = p
				}
				curr.Args = append(curr.Args, a[1:])
			}
		} else {
			if curr.Path == "" {
				curr.Path = a
				p, err := exec.LookPath(curr.Path)
				if err == nil {
					curr.Path = p
				}
				curr.Args = append(curr.Args, a[1:])
			} else {
				curr.Args = append(curr.Args, a)
			}
			//rog.Printf("curr: %T", curr)
		}
	}

	//rog.Print("oscmds", len(oscmds))

	oscmds[0].Args = append(oscmds[0].Args, args[1:]...)

	oscmds[0].Stdin = os.Stdin
	oscmds[0].Stderr = os.Stderr
	oscmds[len(oscmds)-1].Stdout = os.Stdout
	oscmds[len(oscmds)-1].Stderr = os.Stderr
	for i := 1; i < len(oscmds); i++ {
		//rog.Print("pipe")
		stdoutPipe, err := oscmds[i-1].StdoutPipe()
		if err != nil {
			fmt.Fprintln(os.Stderr, "stdoutPipe: %v", err)
			return
		}
		oscmds[i].Stdin = stdoutPipe
		oscmds[i].Stderr = os.Stderr
	}
	//rog.Printf("oscmds:%#v", oscmds)

	for i := range oscmds {
		//rog.Printf("starting %#v", c)
		err = oscmds[i].Start()
		if err != nil {
			fmt.Fprintf(os.Stderr, "start: %v\n", err)
			return
		}
	}

	for i := range oscmds {
		err = oscmds[i].Wait()
		//rog.Print(oscmds[i], err)
		if i == len(oscmds)-1 && err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				os.Exit(exit.ExitCode())
			}
		}
	}
}

func addCommand(config *Config, name, path string, args []string) {
	idx := -1
	for i, c := range config.Commands {
		if c.Name == name {
			idx = i
			break
		}
	}
	if idx != -1 {
		config.Commands[idx].Path = path
		config.Commands[idx].Args = args
	} else {
		config.Commands = append(config.Commands, Command{
			Name: name,
			Path: path,
			Args: args,
		})
	}

	sort.Slice(config.Commands, func(i, j int) bool {
		return config.Commands[i].Name < config.Commands[j].Name
	})
}

func removeCommand(config *Config, name string) {
	var idx int
	for i, c := range config.Commands {
		if c.Name == name {
			idx = i
		}
	}
	config.Commands = append(config.Commands[:idx], config.Commands[idx+1:]...)
}

func findCommand(config *Config, name string) *Command {
	for _, c := range config.Commands {
		if c.Name == name {
			cc := c
			return &cc
		}
	}
	return nil
}

func determineConfigPath() string {
	ep, err := os.Executable()
	if err != nil {
		return ""
	}

	ext := filepath.Ext(ep)
	if ext == "" {
		ep += ".json"
	} else {
		ep = ep[:len(ep)-len(ext)] + ".json"
	}

	info, err := os.Stat(ep)
	if err == nil && !info.IsDir() {
		return ep
	}
	// remember the ep

	// if ep not found, search for config dir

	configname := filepath.Base(ep)

	cp, err := os.UserConfigDir()
	if err != nil {
		return ep
	}

	cp = filepath.Join(cp, userConfigFolder, configname)

	info, err = os.Stat(cp)
	if err == nil && !info.IsDir() {
		return cp
	}

	return ep
}

func loadConfig(configPath string) (*Config, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return &Config{}, nil
	}
	content, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	file.Close()

	var config Config
	err = json.Unmarshal(content, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func saveConfig(configPath string, config *Config) error {
	content, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	err = os.WriteFile(configPath, content, os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}
