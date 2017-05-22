// Package api provides implementation of aptly REST API
package api

import (
	"fmt"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/smira/aptly/aptly"
	"github.com/smira/aptly/deb"
	"github.com/smira/aptly/query"
	"github.com/smira/aptly/task"
)

// Lock order acquisition (canonical):
//  1. RemoteRepoCollection
//  2. LocalRepoCollection
//  3. SnapshotCollection
//  4. PublishedRepoCollection

// GET /api/version
func apiVersion(c *gin.Context) {
	c.JSON(200, gin.H{"Version": aptly.Version})
}

type dbRequestKind int

const (
	acquiredb dbRequestKind = iota
	releasedb
)

type dbRequest struct {
	kind dbRequestKind
	err  chan<- error
}

var dbRequests chan dbRequest

// Acquire database lock and release it when not needed anymore.
//
// Should be run in a goroutine!
func acquireDatabase() {
	clients := 0
	for request := range dbRequests {
		var err error

		switch request.kind {
		case acquiredb:
			if clients == 0 {
				err = context.ReOpenDatabase()
			}

			request.err <- err

			if err == nil {
				clients++
			}
		case releasedb:
			clients--
			if clients == 0 {
				err = context.CloseDatabase()
			} else {
				err = nil
			}

			request.err <- err
		}
	}
}

// Should be called before database access is needed in any api call.
// Happens per default for each api call. It is important that you run
// runTaskInBackground to run a task which accquire database.
// Important do not forget to defer to releaseDatabaseConnection
func acquireDatabaseConnection() error {
	if dbRequests == nil {
		return nil
	}

	errCh := make(chan error)
	dbRequests <- dbRequest{acquiredb, errCh}

	return <-errCh
}

// Release database connection when not needed anymore
func releaseDatabaseConnection() error {
	if dbRequests == nil {
		return nil
	}

	errCh := make(chan error)
	dbRequests <- dbRequest{releasedb, errCh}
	return <-errCh
}

// runs tasks in background. Acquires database connection first.
func runTaskInBackground(name string, resources []string, proc task.Process) (task.Task, *task.ResourceConflictError) {
	return context.TaskList().RunTaskInBackground(name, resources, func(out *task.Output, detail *task.Detail) error {
		err := acquireDatabaseConnection()

		if err != nil {
			return err
		}

		defer releaseDatabaseConnection()
		return proc(out, detail)
	})
}

// Common piece of code to show list of packages,
// with searching & details if requested
func showPackages(c *gin.Context, reflist *deb.PackageRefList, collectionFactory *deb.CollectionFactory) {
	result := []*deb.Package{}

	list, err := deb.NewPackageListFromRefList(reflist, collectionFactory.PackageCollection(), nil)
	if err != nil {
		c.Fail(404, err)
		return
	}

	queryS := c.Request.URL.Query().Get("q")
	if queryS != "" {
		q, err := query.Parse(c.Request.URL.Query().Get("q"))
		if err != nil {
			c.Fail(400, err)
			return
		}

		withDeps := c.Request.URL.Query().Get("withDeps") == "1"
		architecturesList := []string{}

		if withDeps {
			if len(context.ArchitecturesList()) > 0 {
				architecturesList = context.ArchitecturesList()
			} else {
				architecturesList = list.Architectures(false)
			}

			sort.Strings(architecturesList)

			if len(architecturesList) == 0 {
				c.Fail(400, fmt.Errorf("unable to determine list of architectures, please specify explicitly"))
				return
			}
		}

		list.PrepareIndex()

		list, err = list.Filter([]deb.PackageQuery{q}, withDeps,
			nil, context.DependencyOptions(), architecturesList)
		if err != nil {
			c.Fail(500, fmt.Errorf("unable to search: %s", err))
			return
		}
	}

	if c.Request.URL.Query().Get("format") == "details" {
		list.ForEach(func(p *deb.Package) error {
			result = append(result, p)
			return nil
		})

		c.JSON(200, result)
	} else {
		c.JSON(200, list.Strings())
	}
}
