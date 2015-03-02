![HidalgoLogo](https://rawgithub.com/xogeny/hidalgo/master/hidalgo.svg)

# Hidalgo

One of the great things about Golang is that you can build statically
linked executables.  As many people have recognized, this makes
deployment of Go apps very easy because no runtime environment is
required.  On top of this, Golang also makes it easy to cross-compile
code for other environments.

Hidalgo is a simple tool that allows you to easily build a Docker
image for your Golang application.  By default, you can simply run:

```
$ hidalgo
```

In the same directory you would run `go build` in.  Hidalgo will
generate a `linux`/`amd64` compiled version of your application and
create a Docker image from that.  Upon completion of the Docker image,
you should see a message like:

```
Successfully built 1ab5dd324158
```

You can then run something like:

```
$ docker run 1ab5dd324158
```

and your Golang app will be up and running.

Now, it is awkward to deal with such a name.  So, when running
`hidalgo` you can do this instead:

```
$ hidalgo -t <tagname>
```

To see this in action, `hidalgo` comes with a same application.  From
the `hidalgo` source directory, you can do this:

```
$ hidalgo -t htest/hello ./examples/hello
```

This will build an image for the `hello` application tag it with
`htest/hello`.  Then the application can be run as:

```
$ docker run htest/hello
```

## Configuration

It turns out that there are a number of options you might want to
specify when running `hidalgo`.  You can see some of them by just
running:

```
$ hidalgo -h
Usage:
  hidalgo [OPTIONS] [Directory]

Application Options:
  -d, --docker=    Docker command (sdocker)
  -t, --tag=       Name to tag image with
  -f, --from=      Docker image to build FROM
  -b, --builddir=  Directory for Docker build
  -k, --keep       Keep Docker build directory
  -v, --verbose    Verbose output
  -n, --dryrun     Suppress docker build

Help Options:
  -h, --help       Show this help message

Arguments:
  Directory:       Directory of Go package to build
```

But there are more configuration options.

By default, when you run `hidalgo` it will resolve the directory that
the package you are attempting to build is stored in.  It will look
for a configuration file named `hidalgo.cfg` in that directory (the
directory where a `go build` would be performed).  It then reads that
file information about:

### Ports

If your application requires ports to be exposed, you can list them in
the `hidalgo.cfg` file, e.g.,

```
port 8080;
```

This is then turned into `EXPOSE` commands in the generated `Dockerfile`.

### Environment Variables

When I build Docker images, I am careful to avoid keeping credential
information in any files that are version controlled or otherwise
distributed.  As such, much of my code relies on environment variables
being specified.  Of course, this can always be done with the `-e`
switch when doing executing `docker run`.  But I like to create images
that are easy to spin up.  So you can specify relevant environment
variables in `hidalgo.cfg` as follows:

```
env AWS_CLIENT_KEY;
env AWS_SECRET_KEY;
```

If those environment variables are set (i.e., in the environment) when
`hidalgo` is run, it will add them to the `Dockerfile` using the `ENV`
directive.  This means they will get "baked" into the `Dockerfile`.
**Note, you should only do this if you will not be publishing the
resulting `Dockerfile`**.  In other words, use this feature with
caution and understand whatever opportunities for "leaking"
credentials might result.

## Docker client

By default, `hidalgo` uses
[`sdocker`](http://github.com/xogeny/sdocker) as the Docker client.
This can be changed on the command line by using the `-d` command line
flag, e.g.,

```
$ hidalgo -d docker
```

The reason I use `sdocker` is that it is effectively a drop-in
replacement for `docker` that includes support for working with remote
Docker hosts via SSH.  Since I do my development work on OSX, this is
a really useful feature which is why I made it the default.  If people
really find this annoying in the future, I'd consider adding some kind
of `~/.hidalgo` file where you could specify your global preferences.

## Installation

To install `hidalgo`, all you should need to do is run:

```
go get github.com/xogeny/hidalgo
```

However, it also requires that you have all the cross-compilation
capabilities built.  If you are using Homebrew, this running something like:

```
brew upgrade go --cross-compile-common
```

I'm not sure about Windows, but I'm sure it isn't very hard.

## Known Issues

I ran across a strange issue when working with Hidalgo.  There are
cases where the Go compiler will raise errors when cross-compiling but
not when performing normal (native) compilations.  This impacts
Hidalgo since it performs cross-compilations behind the scenes.  The
specific issue I ran into (which is discussed
[here](https://groups.google.com/forum/#!topic/golang-nuts/XYoBvsrvyRA)
and
[here](https://groups.google.com/forum/#!topic/golang-nuts/rPhnLR9OmLI))
was related to method declarations in separate files from their
receiver types.  According to the
[Golang specification](https://golang.org/ref/spec#Method_declarations)
this is legal, but it generates an error during cross compilation.  So
if you get an error that looks something like this (but only when
using Hidalgo or performing cross-compilation):

```
.../FileWithMethod.go:10: undefined: ReceiverType
```

...you may have run into it.  Please note, this is not a bug in
Hidalgo (you can confirm this by simply setting `GOOS` to `linux` and
`GOARCH` to `amd64` and trying to do a `go build` yourself.

## References

A similar effort to create a Docker container that builds Docker
containers for Golang applications can be found
[here](https://registry.hub.docker.com/u/centurylink/golang-builder/).
I only found this after I created Hidalgo and my goals were slightly
different so I don't really think either one makes the other
redundant.
