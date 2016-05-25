// Copyright 2015 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Go-import-redirector is an HTTP server for a custom Go import domain.
// It responds to requests in a given import path root with a meta tag
// specifying the source repository for the ``go get'' command and an
// HTML redirect to the godoc.org documentation page for that package.
//
// Usage:
//
//	go-import-redirector [-addr address] [-tls] [-vcs sys] <import> <repo>
//
// Go-import-redirector listens on address (default ``:80'')
// and responds to requests for URLs in the given import path root
// with one meta tag specifying the given source repository for ``go get''
// and another meta tag causing a redirect to the corresponding
// godoc.org documentation page.
//
// For example, if invoked as:
//
//	go-import-redirector 9fans.net/go https://github.com/9fans/go
//
// then the response for 9fans.net/go/acme/editinacme will include these tags:
//
//	<meta name="go-import" content="9fans.net/go git https://github.com/9fans/go">
//	<meta http-equiv="refresh" content="0; url=https://godoc.org/9fans.net/go/acme/editinacme">
//
// If both <import> and <repo> end in /*, the corresponding path element
// is taken from the import path and substituted in repo on each request.
// For example, if invoked as:
//
//	go-import-redirector rsc.io/* https://github.com/rsc/*
//
// then the response for rsc.io/x86/x86asm will include these tags:
//
//	<meta name="go-import" content="rsc.io/x86 git https://github.com/rsc/x86">
//	<meta http-equiv="refresh" content="0; url=https://godoc.org/rsc.io/x86/x86asm">
//
// Note that the wildcard element (x86) has been included in the Git repo path.
//
// The -addr option specifies the HTTP address to serve (default ``:http'').
//
// The -tls option causes go-import-redirector to serve HTTPS on port 443,
// loading an X.509 certificate and key pair from files in the current directory
// named after the host in the import path with .crt and .key appended
// (for example, rsc.io.crt and rsc.io.key).
// Like for http.ListenAndServeTLS, the certificate file should contain the
// concatenation of the server's certificate and the signing certificate authority's certificate.
//
// The -vcs option specifies the version control system, git, hg, or svn (default ``git'').
//
// Deployment on Google Cloud Platform
//
// For the case of a redirector for an entire domain (such as rsc.io above),
// the Makefile in this directory contains recipes to deploy a trivial VM running
// just this program, using a static IP address that can be loaded into the
// DNS configuration for the target domain.
//
package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
)

var (
	addr       = flag.String("addr", ":http", "serve http on `address`")
	serveTLS   = flag.Bool("tls", false, "serve https on :443")
	vcs        = flag.String("vcs", "git", "set version control `system`")
	godoc      = flag.String("godoc", "https://godoc.org/", "godoc redirect address")
	importPath string
	repoPath   string
	wildcard   bool
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: go-import-redirector <import> <repo>\n")
	fmt.Fprintf(os.Stderr, "options:\n")
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "examples:\n")
	fmt.Fprintf(os.Stderr, "\tgo-import-redirector rsc.io/* https://github.com/rsc/*\n")
	fmt.Fprintf(os.Stderr, "\tgo-import-redirector 9fans.net/go https://github.com/9fans/go\n")
	os.Exit(2)
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("go-import-redirector: ")
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() != 2 {
		flag.Usage()
	}
	importPath = flag.Arg(0)
	repoPath = flag.Arg(1)
	// if !strings.Contains(repoPath, "://") {
	// 	log.Fatal("repo path must be full URL")
	// }
	if strings.HasSuffix(importPath, "/*") != strings.HasSuffix(repoPath, "/*") {
		log.Fatal("either both import and repo must have /* or neither")
	}
	if strings.HasSuffix(importPath, "/*") {
		wildcard = true
		importPath = strings.TrimSuffix(importPath, "/*")
		repoPath = strings.TrimSuffix(repoPath, "/*")
	}
	http.HandleFunc(strings.TrimSuffix(importPath, "/")+"/", redirect)
	if *serveTLS {
		host := importPath
		if i := strings.Index(host, "/"); i >= 0 {
			host = host[:i]
		}
		go func() {
			log.Fatal(http.ListenAndServeTLS(":https", host+".crt", host+".key", nil))
		}()
	}
	log.Fatal(http.ListenAndServe(*addr, nil))
}

var tmpl = template.Must(template.New("main").Parse(`<!DOCTYPE html>
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=utf-8"/>
<meta name="go-import" content="{{.ImportRoot}} {{.VCS}} {{.VCSRoot}}">
<meta http-equiv="refresh" content="0; url={{.GoDoc}}">
</head>
<body>
Nothing to see here; <a href="{{.GoDoc}}">move along</a>.
</body>
</html>
`))

type data struct {
	ImportRoot string
	VCS        string
	VCSRoot    string
	GoDoc      template.URL
}

func redirect(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimSuffix(req.Host+req.URL.Path, "/")
	var importRoot, repoRoot, suffix string
	if wildcard {
		if path == importPath {
			http.Redirect(w, req, *godoc+importPath, 302)
			return
		}
		if !strings.HasPrefix(path, importPath+"/") {
			http.NotFound(w, req)
			return
		}
		elem := path[len(importPath)+1:]
		if i := strings.Index(elem, "/"); i >= 0 {
			elem, suffix = elem[:i], elem[i:]
		}
		importRoot = importPath + "/" + elem
		repoRoot = repoPath + "/" + elem
	} else {
		if path != importPath && !strings.HasPrefix(path, importPath+"/") {
			http.NotFound(w, req)
			return
		}
		importRoot = importPath
		repoRoot = repoPath
		suffix = path[len(importPath):]
	}
	goDocURL := template.URL(*godoc + importRoot + suffix)
	d := &data{
		ImportRoot: importRoot,
		VCS:        *vcs,
		VCSRoot:    repoRoot,
		GoDoc:      goDocURL,
	}
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, d)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Write(buf.Bytes())
}
