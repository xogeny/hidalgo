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

// This is the (Denada) grammar for the configuration file.
// N.B. - Currently, the file directive is ignored.
const configGrammar = `
env _ "env*";

file _ "file*";

port [0-9]+ "port*";
`

// This is the template for the Dockerfile that will be generated
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

# Expose any ports required
{{range $value := .ports}}
EXPOSE {{$value}}
{{end}}

# Run the executable
CMD ["/usr/local/bin/server_linux64"]
`

// Options is a structure used to describe the various command line
// options.
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

// Config is a structure that contains information parsed from the configuration
// file.  This is information that would be otherwise inconvenient to include
// in the command line because it is either repetitive (always required) or extensive
// (involves a lot of information).
type Config struct {
	Files []string
	Env   []string
	Ports []int
}

// The cmdString function generates a textual representation of a
// exec.Cmd instance.
func cmdString(cmd *exec.Cmd) string {
	return fmt.Sprintf("%s %s", cmd.Path, strings.Join(cmd.Args[1:], " "))
}

// The parseConfig function walks the elements in the config file and uses
// them to populate an instance of the Config structure.
func parseConfig(config denada.ElementList) (Config, error) {
	// Initial configuration is empty
	ret := Config{}

	// Look for any elements that match the "env" rule and add their
	// name to the Config.Env array
	for _, e := range config.OfRule("env", false) {
		ret.Env = append(ret.Env, e.Name)
	}

	// Look for any elements that match the "port" rule, turn their
	// name into a number (checking for proper values) and then add
	// them to the Config.Ports array
	for _, e := range config.OfRule("port", false) {
		num, err := strconv.ParseInt(e.Name, 0, 0)
		if err != nil || num < 1 || num > 65535 {
			return ret, fmt.Errorf("Invalid port number: %s", e.Name)
		}
		ret.Ports = append(ret.Ports, int(num))
	}

	// Look for any elements that match the "file" rule and add them
	// to the Config.Files array.
	for _, e := range config.OfRule("file", false) {
		ret.Files = append(ret.Files, e.Name)
	}

	// Return all the data that was collected
	return ret, nil
}

// The packageName function takes the name of a directory (potentially
// relative) and returns the name of the Go package it points to.  At
// some point, I'd like to use the go/parser package to come up with
// something a bit more formal and robust.  This function simply expands
// the directory to an absolute path, looks for GOPATH as a substring
// and then trims the GOPATH prefix.
func packageName(dir string) (string, string, error) {
	// Get absolute name of directory
	adir, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}

	// Follow any symbolic links
	act, err := filepath.EvalSymlinks(adir)
	if err != nil {
		return "", "", err
	}

	// Get the current value of GOPATH
	gp := os.Getenv("GOPATH")
	if gp == "" {
		return "", "", fmt.Errorf("No GOPATH specified")
	}

	// Add src to GOPATH
	sdir := path.Join(gp, "src")

	// Check if the target directory existsin in GOPATH/src
	if filepath.HasPrefix(act, sdir) {
		// If so, get the relative path within GOPATH/src...
		pname, err := filepath.Rel(sdir, act)
		if err != nil {
			return "", "", err
		}
		// ...and return it as the package name (along with the full path)
		return act, pname, nil
	} else {
		return "", "", fmt.Errorf("Directory %s not inside %s", act, sdir)
	}
}

// The addIf function looks to see if the named environment variable is
// actually present in the current environment (i.e., os.Getenv returns
// something other than "").  If so, it adds it to the list of environement
// variables that are given a value in the generated Dockerfile
func addIf(name string, env map[string]string) bool {
	if os.Getenv(name) != "" {
		env[name] = os.Getenv(name)
		return true
	}
	return false
}

// This is (obviously), the entry point for the tool
func main() {
	// Get command line options
	var Options Options
	parser := flags.NewParser(&Options, flags.Default)

	if _, err := parser.Parse(); err != nil {
		os.Exit(1)
	}

	// Now determine package to be built
	// We assume they mean the current directory...
	pdir := "."
	if Options.Positional.Directory != "" {
		// ...unless they specified something explicitly
		pdir = Options.Positional.Directory
	}

	// Get the absolute directory path and package name
	apdir, name, err := packageName(pdir)
	if err != nil {
		log.Printf("Error determining package name: %v", err)
		os.Exit(1)
	}

	if Options.Verbose {
		log.Printf("Package name: %s", name)
	}

	// Parse the *grammar* for the configuration file
	grammar, err := denada.ParseString(configGrammar)
	if err != nil {
		// This should not happen
		log.Printf("Internal error in grammar specification: %v", err)
		os.Exit(1)
	}

	// This is the name of the configuration file
	cfile := path.Join(apdir, "hidalgo.cfg")

	// Assume no configuration options
	conf := denada.ElementList{}

	// ...unless the configuration file exists
	if _, err := os.Stat(cfile); err == nil {
		// In that case, we parse it to determine the value for 'conf'
		conf, err = denada.ParseFile(cfile)
		if err != nil {
			log.Printf("Error reading configuration file %s: %v", cfile, err)
			os.Exit(1)
		}
		if Options.Verbose {
			log.Printf("Configuration file: %s", cfile)
		}
	}

	// Check the parsed configuration against the grammar to make
	// sure we know exactly what is in it.
	err = denada.Check(conf, grammar, false)
	if err != nil {
		log.Printf("Error in configuration: %v", err)
		os.Exit(2)
	}

	// Now go through the (grammatically valid) configuration AST
	// and extract the information we need.
	config, err := parseConfig(conf)
	if err != nil {
		log.Printf("Error in configuration: %v", err)
	}

	// Assume that we will use the explicitly provided build directory...
	dir := Options.Build

	// ...unless they didn't specify one.
	if dir == "" {
		// In that case, we create a temporary directory...
		dir, err = ioutil.TempDir("", "hidalgo")
		if err != nil {
			log.Printf("Error: Cannot create temporary directory")
			os.Exit(2)
		}
		// ...which is removed when we are all done.
		defer os.RemoveAll(dir)
	} else {
		// Make sure the directory they specified exists and if it
		// doesn't, make it.
		err = os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			log.Printf("Error: Unable to create directory %s: %v", dir, err)
			os.Exit(2)
		}
	}

	if Options.Verbose {
		log.Printf("Build directory: %s", dir)
	}

	// Now change to the build directory
	err = os.Chdir(dir)
	if err != nil {
		log.Printf("Error: Cannot change to build directory %s", dir)
		os.Exit(2)
	}

	if Options.Verbose {
		log.Printf("Building directory: %s", dir)
	}

	// Specify the values of GOOS and GOARCH to be 64 bit linux
	os.Setenv("GOOS", "linux")
	os.Setenv("GOARCH", "amd64")

	// Build the static Go executable
	build := exec.Command("go", "build", "-o", "server_linux64", name)

	output, err := build.CombinedOutput()
	if err != nil {
		log.Printf("Error running cmd '%s':\n%s\n%v", cmdString(build), output, err)
		os.Exit(3)
	}

	if Options.Verbose {
		log.Printf("Build of %s successful", name)
	}

	// Assume we will start from the "scratch" Docker image...
	from := "scratch"
	if Options.From != "" {
		// ...unless one is explicitly specified
		from = Options.From
	}

	// Build the Dockerfile template
	t1 := template.New("Dockerfile")
	t, err := t1.Parse(dockerTemplate)
	if err != nil {
		log.Printf("Error parsing Dockerfile template: %v", err)
		os.Exit(4)
	}

	// Open a new file to write the Dockerfile contents into
	dfile, err := os.Create("Dockerfile")
	if err != nil {
		log.Printf("Unable to create Dockerfile in %s: %v", dir, err)
		os.Exit(4)
	}

	// Build up the context information for evaluating the template
	context := map[string]interface{}{}
	// Start with empty environment variable definitions
	env := map[string]string{}
	// And then add any relevant environment variables that are in the current
	// environment.
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
	// Add those environment variables to the template context
	context["env"] = env

	// Now add any ports that need to be exposed.
	context["ports"] = config.Ports
	if Options.Verbose {
		log.Printf("Exported ports: %v", config.Ports)
	}

	// Now specify the Docker image that we will build our image from
	context["from"] = from
	if Options.Verbose {
		log.Printf("Base Docker image to build FROM: %s", from)
	}

	// Execute the template and write it to the Dockerfile
	err = t.Execute(dfile, context)
	if err != nil {
		log.Printf("Error rendering template: %v", err)
		os.Exit(5)
	}

	// If the user specified verbose output, dump the Dockerfile
	// to os.Stdout as well
	if Options.Verbose {
		log.Printf("===== Dockerfile =====")
		t.Execute(os.Stdout, context)
		log.Printf("===== Dockerfile =====")
	}

	// Get the docker client name from the command line options
	// (sdocker is the default)
	dcmd := Options.Docker
	if dcmd == "" {
		// If somehow not specified, throw an error
		log.Printf("Missing Docker command")
		os.Exit(5)
	}

	if Options.Verbose {
		log.Printf("Docker command used: %s", dcmd)
	}

	// Check to see if this was just a dry run
	if !Options.Dry {
		// If not, time to build the docker image.

		// First, we determine the command line arguments to the
		// docker build command
		// TODO: Use go/parser to determine package name and auto-generate
		// a tag (e.g., hidalgo/<pkgname>
		args := []string{"build", "-"}
		if Options.Tag != "" {
			args = []string{"build", "-t", Options.Tag, "-"}
		}
		sbuild := exec.Command(dcmd, args...)

		if Options.Verbose {
			log.Printf("  Complete build command: '%s'", cmdString(sbuild))
		}

		// We also need to tar up our build directory to pass it to
		// Docker.  This handles the case where the build is actually
		// being performed on a remote machine.
		tar := exec.Command("tar", "zcf", "-", ".")

		if Options.Verbose {
			log.Printf("  Complete tar command: '%s'", cmdString(tar))
		}

		// Create a pipe from tar to build
		reader, writer := io.Pipe()

		// push first command output to writer
		tar.Stdout = writer

		// read from first command output
		sbuild.Stdin = reader
		sbuild.Stdout = os.Stdout

		// Start archiving the directory
		tar.Start()

		// Start the build
		sbuild.Start()

		// Wait until the archiving is done
		terr := tar.Wait()

		// Then close the writer
		writer.Close()

		// Then wait until the build is done
		serr := sbuild.Wait()

		// Check for errors
		if terr != nil {
			log.Printf("Error generating archive: %v", terr)
			os.Exit(3)
		}
		if serr != nil {
			log.Printf("Error performing build: %v", err)
			os.Exit(3)
		}

		// It must have worked!
		if Options.Verbose {
			log.Printf("Image built!")
		}
	}
}
