// Copyright 2016 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package godoc serves redirects to pkg.go.dev, including ``go get'' headers.
package godoc

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"strings"
)

var tmpl = template.Must(template.New("main").Parse(`<!DOCTYPE html>
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=utf-8"/>
<meta name="go-import" content="{{.ImportRoot}} {{.VCS}} {{.VCSRoot}}">
<meta http-equiv="refresh" content="0; url=https://pkg.go.dev/{{.ImportRoot}}{{.Suffix}}">
</head>
<body>
Redirecting to docs at <a href="https://pkg.go.dev/{{.ImportRoot}}{{.Suffix}}">pkg.go.dev/{{.ImportRoot}}{{.Suffix}}</a>...
</body>
</html>
`))

type data struct {
	ImportRoot string
	VCS        string
	VCSRoot    string
	Suffix     string
}

// Redirect returns an HTTP handler that redirects requests for the tree rooted at importPath
// to pkg.go.dev pages for those import paths.
// The redirections include headers directing ``go get'' to satisfy the
// imports by checking out code from repoPath using the given version control system.
//
// As a special case, if both importPath and repoPath end in /*, then the matching
// element in the importPath is substituted into the repoPath specified for ``go get.''
//
// For example, the top-level directories at rsc.io maps to individual GitHub repositories
// under github.com/rsc, by using:
//
//	http.Handle("/", godoc.Redirect("git", "rsc.io/*", "https://github.com/rsc/*"))
//
// As another example, import paths starting with 9fans.net/go map to the
// single GitHub repository github.com/9fans/go, by using:
//
//	http.Handle("/go/", godoc.Redirect("git", "9fans.net/go", "https://github.com/9fans/go"))
//
func Redirect(vcs, importPath, repoPath string) http.Handler {
	wildcard := false
	if strings.HasSuffix(importPath, "/*") && strings.HasSuffix(repoPath, "/*") {
		wildcard = true
		importPath = strings.TrimSuffix(importPath, "/*")
		repoPath = strings.TrimSuffix(repoPath, "/*")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/.ping") {
			fmt.Fprintf(w, "pong")
			return
		}
		path := strings.TrimSuffix(strings.TrimSuffix(req.Host+req.URL.Path, "@latest"), "/")
		var importRoot, repoRoot, suffix string
		if wildcard {
			if path == importPath {
				http.Redirect(w, req, "https://pkg.go.dev/"+importPath, 302)
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
		d := &data{
			ImportRoot: importRoot,
			VCS:        vcs,
			VCSRoot:    repoRoot,
			Suffix:     suffix,
		}
		var buf bytes.Buffer
		err := tmpl.Execute(&buf, d)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Write(buf.Bytes())
	})
}
