package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/jessevdk/go-flags"
	"github.com/xogeny/denada-go"
)

const configGrammar = `
env _ "env*";

file _ "file*";

port [0-9]+ "port*";
`

const dockerTemplate = `
# Start from a Debian image with the latest version of Go installed
# and a workspace (GOPATH) configured at /go.
FROM {{.from}}

# Copy local executable to image
ADD server_linux64 /usr/local/bin/server_linux64

# Environment variable values available at *build* time
# (if you don't see variables you expect, either define them
# when running hidalgo OR specify them when running the image)
{{range $key, $value := .env }}
ENV {{$key}} {{$value}}
{{end}}

# Run the executable
ENTRYPOINT /usr/local/bin/server_linux64

# Expose any ports required
{{range $value := .ports}}
EXPOSE {{$value}}
{{end}}
`

type Options struct {
	Positional struct {
		Directory string `description:"Directory of Go package to build"`
	} `positional-args:"true"`

	Docker  string `short:"d" long:"docker" description:"Docker command" default:"sdocker"`
	Tag     string `short:"t" long:"tag" description:"Name to tag image with"`
	From    string `short:"f" long:"from" description:"Docker image to build FROM"`
	Build   string `short:"b" long:"builddir" description:"Directory for Docker build"`
	Keep    bool   `short:"k" long:"keep" description:"Keep Docker build directory"`
	Verbose bool   `short:"v" long:"verbose" description:"Verbose output"`
	Dry     bool   `short:"n" long:"dryrun" description:"Suppress docker build"`
}

type Config struct {
	Files []string
	Env   []string
	Ports []int
}

func cmdString(cmd *exec.Cmd) string {
	return fmt.Sprintf("%s %s", cmd.Path, strings.Join(cmd.Args[1:], " "))
}

func parseConfig(config denada.ElementList) (Config, error) {
	ret := Config{}

	for _, e := range config.OfRule("env", false) {
		ret.Env = append(ret.Env, e.Name)
	}

	for _, e := range config.OfRule("port", false) {
		num, err := strconv.ParseInt(e.Name, 0, 0)
		if err != nil || num < 1 || num > 65535 {
			return ret, fmt.Errorf("Invalid port number: %s", e.Name)
		}
		ret.Ports = append(ret.Ports, int(num))
	}

	for _, e := range config.OfRule("file", false) {
		ret.Files = append(ret.Files, e.Name)
	}

	return ret, nil
}

func packageName(dir string) (string, string, error) {
	adir, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}

	act, err := filepath.EvalSymlinks(adir)
	if err != nil {
		return "", "", err
	}

	gp := os.Getenv("GOPATH")
	if gp == "" {
		return "", "", fmt.Errorf("No GOPATH specified")
	}

	sdir := path.Join(gp, "src")

	if filepath.HasPrefix(act, sdir) {
		pname, err := filepath.Rel(sdir, act)
		if err != nil {
			return "", "", err
		}
		return act, pname, nil
	} else {
		return "", "", fmt.Errorf("Directory %s not inside %s", act, sdir)
	}
}

func addIf(name string, env map[string]string) bool {
	if os.Getenv(name) != "" {
		env[name] = os.Getenv(name)
		return true
	}
	return false
}

func main() {
	// Get command line options
	var Options Options
	parser := flags.NewParser(&Options, flags.Default)

	if _, err := parser.Parse(); err != nil {
		os.Exit(1)
	}

	// Now determine package to be built
	pdir := "."
	if Options.Positional.Directory != "" {
		pdir = Options.Positional.Directory
	}

	apdir, name, err := packageName(pdir)
	if err != nil {
		log.Printf("Error determining package name: %v", err)
		os.Exit(1)
	}

	if Options.Verbose {
		log.Printf("Package name: %s", name)
	}

	cfile := path.Join(apdir, "hidalgo.cfg")

	conf := denada.ElementList{}
	grammar, err := denada.ParseString(configGrammar)
	if err != nil {
		log.Printf("Internal error in grammar specification: %v", err)
		os.Exit(1)
	}

	if _, err := os.Stat(cfile); err == nil {
		conf, err = denada.ParseFile(cfile)
		if err != nil {
			log.Printf("Error reading configuration file %s: %v", cfile, err)
			os.Exit(1)
		}
		if Options.Verbose {
			log.Printf("Configuration file: %s", cfile)
		}
	}

	err = denada.Check(conf, grammar, false)
	if err != nil {
		log.Printf("Error in configuration: %v", err)
		os.Exit(2)
	}

	config, err := parseConfig(conf)
	if err != nil {
		log.Printf("Error in configuration: %v", err)
	}

	dir := Options.Build

	if dir == "" {
		dir, err = ioutil.TempDir("", "hidalgo")
		if err != nil {
			log.Printf("Error: Cannot create temporary directory")
			os.Exit(2)
		}
		defer os.RemoveAll(dir)
	} else {
		err = os.MkdirAll(dir, os.ModePerm)
	}

	if Options.Verbose {
		log.Printf("Build directory: %s", dir)
	}

	err = os.Chdir(dir)
	if err != nil {
		log.Printf("Error: Cannot change to build directory %s", dir)
		os.Exit(2)
	}

	if Options.Verbose {
		log.Printf("Building directory: %s", dir)
	}

	os.Setenv("GOOS", "linux")
	os.Setenv("GOARCH", "amd64")

	build := exec.Command("go", "build", "-o", "server_linux64", name)
	err = build.Run()
	if err != nil {
		log.Printf("Error running cmd '%s': %v", cmdString(build), err)
		os.Exit(3)
	}

	if Options.Verbose {
		log.Printf("Build of %s successful", name)
	}

	// TODO: I'd like to make this work with "scratch" at some point
	from := "busybox"
	if Options.From != "" {
		from = Options.From
	}

	t1 := template.New("Dockerfile")
	t, err := t1.Parse(dockerTemplate)
	if err != nil {
		log.Printf("Error parsing Dockerfile template: %v", err)
		os.Exit(4)
	}

	dfile, err := os.Create("Dockerfile")
	if err != nil {
		log.Printf("Unable to create Dockerfile in %s: %v", dir, err)
		os.Exit(4)
	}

	context := map[string]interface{}{}
	env := map[string]string{}
	for _, e := range config.Env {
		added := addIf(e, env)
		if Options.Verbose {
			if added {
				log.Printf("  Environment variable %s added to Dockerfile", e)
			} else {
				log.Printf("  Environment variable %s not added to Dockerfile", e)
			}
		}
	}
	context["env"] = env

	context["ports"] = config.Ports
	if Options.Verbose {
		log.Printf("Exported ports: %v", config.Ports)
	}

	context["from"] = from
	if Options.Verbose {
		log.Printf("Base Docker image to build FROM: %s", from)
	}

	err = t.Execute(dfile, context)
	if err != nil {
		log.Printf("Error rendering template: %v", err)
		os.Exit(5)
	}

	if Options.Verbose {
		log.Printf("===== Dockerfile =====")
		t.Execute(os.Stdout, context)
		log.Printf("===== Dockerfile =====")
	}

	dcmd := Options.Docker
	if dcmd == "" {
		os.Exit(5)
	}

	if Options.Verbose {
		log.Printf("Docker command used: %s", dcmd)
	}

	if !Options.Dry {
		// TODO: Use go/parser to determine package name and auto-generate
		// a tag (e.g., hidalgo/<pkgname>
		args := []string{"build", "-"}
		if Options.Tag != "" {
			args = []string{"build", "-t", Options.Tag, "-"}
		}
		tar := exec.Command("tar", "zcf", "-", ".")
		sbuild := exec.Command(dcmd, args...)

		reader, writer := io.Pipe()

		// push first command output to writer
		tar.Stdout = writer

		// read from first command output
		sbuild.Stdin = reader
		sbuild.Stdout = os.Stdout

		tar.Start()
		sbuild.Start()
		terr := tar.Wait()
		writer.Close()
		serr := sbuild.Wait()

		if terr != nil {
			log.Printf("Error generating archive: %v", terr)
			os.Exit(3)
		}

		if serr != nil {
			log.Printf("Error performing build: %v", err)
			os.Exit(3)
		}

		if Options.Verbose {
			log.Printf("Image built!")
		}
	}
}
