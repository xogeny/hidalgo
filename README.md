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

This will build an image for the `hello` application tag it with `htest/hello`.

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

## References

A similar effort to create a Docker container that builds Docker
containers for Golang applications can be found
[here](https://registry.hub.docker.com/u/centurylink/golang-builder/).
I only found this after I created Hidalgo and my goals were slightly
different so I don't really think either one makes the other
redundant.
