package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
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
        Windows: %appdata%\faker\` + appname + `.json
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

	f := determineConfigPath()

	config, err := loadConfig(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	if len(args) < 1 {
		fmt.Println("commands:")
		for _, c := range config.Commands {
			fmt.Printf("\t%v\t%v %v\n", c.Name, c.Path, c.Args)
		}

		fmt.Println("")
		fmt.Println("config:", f)

		return
	}

	if c.Add {
		n := args[0]
		p := args[1]
		a := args[2:]

		addCommand(config, n, p, a)
		err := saveConfig(f, config)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		return
	}

	if c.Remove {
		n := args[0]
		removeCommand(config, n)

		err := saveConfig(f, config)
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

	oscmd := exec.Command(fcmd.Path, append(fcmd.Args, args[1:]...)...)
	oscmd.Stdin = os.Stdin
	oscmd.Stdout = os.Stdout
	oscmd.Stderr = os.Stderr
	err = oscmd.Run()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
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

func loadConfig(f string) (*Config, error) {
	file, err := os.Open(f)
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

func saveConfig(f string, config *Config) error {
	content, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	err = os.WriteFile(f, content, os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}
