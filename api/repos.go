package api

import (
	"fmt"
	"strings"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/smira/aptly/aptly"
	"github.com/smira/aptly/database"
	"github.com/smira/aptly/deb"
	"github.com/smira/aptly/task"
	"github.com/smira/aptly/utils"
)

// GET /api/repos
func apiReposList(c *gin.Context) {
	result := []*deb.LocalRepo{}

	collectionFactory := context.NewCollectionFactory()
	collection := collectionFactory.LocalRepoCollection()
	collection.ForEach(func(r *deb.LocalRepo) error {
		result = append(result, r)
		return nil
	})

	c.JSON(200, result)
}

// POST /api/repos
func apiReposCreate(c *gin.Context) {
	var b struct {
		Name                string `binding:"required"`
		Comment             string
		DefaultDistribution string
		DefaultComponent    string
	}

	if !c.Bind(&b) {
		return
	}

	repo := deb.NewLocalRepo(b.Name, b.Comment)
	repo.DefaultComponent = b.DefaultComponent
	repo.DefaultDistribution = b.DefaultDistribution

	collectionFactory := context.NewCollectionFactory()
	collection := collectionFactory.LocalRepoCollection()
	err := collection.Add(repo)
	if err != nil {
		c.Fail(400, err)
		return
	}

	c.JSON(201, repo)
}

// PUT /api/repos/:name
func apiReposEdit(c *gin.Context) {
	var b struct {
		Comment             *string
		DefaultDistribution *string
		DefaultComponent    *string
	}

	if !c.Bind(&b) {
		return
	}

	collectionFactory := context.NewCollectionFactory()
	collection := collectionFactory.LocalRepoCollection()

	repo, err := collection.ByName(c.Params.ByName("name"))
	if err != nil {
		c.Fail(404, err)
		return
	}

	if b.Comment != nil {
		repo.Comment = *b.Comment
	}
	if b.DefaultDistribution != nil {
		repo.DefaultDistribution = *b.DefaultDistribution
	}
	if b.DefaultComponent != nil {
		repo.DefaultComponent = *b.DefaultComponent
	}

	err = collection.Update(repo)
	if err != nil {
		c.Fail(500, err)
		return
	}

	c.JSON(200, repo)
}

// GET /api/repos/:name
func apiReposShow(c *gin.Context) {
	collectionFactory := context.NewCollectionFactory()
	collection := collectionFactory.LocalRepoCollection()

	repo, err := collection.ByName(c.Params.ByName("name"))
	if err != nil {
		c.Fail(404, err)
		return
	}

	c.JSON(200, repo)
}

// DELETE /api/repos/:name
func apiReposDrop(c *gin.Context) {
	force := c.Request.URL.Query().Get("force") == "1"
	name := c.Params.ByName("name")

	collectionFactory := context.NewCollectionFactory()
	collection := collectionFactory.LocalRepoCollection()
	snapshotCollection := collectionFactory.SnapshotCollection()
	publishedCollection := collectionFactory.PublishedRepoCollection()

	repo, err := collection.ByName(name)
	if err != nil {
		c.Fail(404, err)
		return
	}

	resources := []string{string(repo.Key())}
	taskName := fmt.Sprintf("Delete repo %s", name)
	task, err := runTaskInBackground(taskName, resources, func(out *task.Output) error {
		published := publishedCollection.ByLocalRepo(repo)
		if len(published) > 0 {
			return fmt.Errorf("unable to drop, local repo is published")
		}

		if !force {
			snapshots := snapshotCollection.ByLocalRepoSource(repo)
			if len(snapshots) > 0 {
				return fmt.Errorf("unable to drop, local repo has snapshots, use ?force=1 to override")
			}
		}

		return collection.Drop(repo)
	})

	if err != nil {
		c.Fail(400, err)
		return
	}

	c.JSON(202, task)
}

// GET /api/repos/:name/packages
func apiReposPackagesShow(c *gin.Context) {
	collectionFactory := context.NewCollectionFactory()
	collection := collectionFactory.LocalRepoCollection()

	repo, err := collection.ByName(c.Params.ByName("name"))
	if err != nil {
		c.Fail(404, err)
		return
	}

	err = collection.LoadComplete(repo)
	if err != nil {
		c.Fail(500, err)
		return
	}

	showPackages(c, repo.RefList(), collectionFactory)
}

// Handler for both add and delete
func apiReposPackagesAddDelete(c *gin.Context, taskNamePrefix string, cb func(list *deb.PackageList, p *deb.Package, out *task.Output) error) {
	var b struct {
		PackageRefs []string
	}

	if !c.Bind(&b) {
		return
	}

	collectionFactory := context.NewCollectionFactory()
	collection := collectionFactory.LocalRepoCollection()

	repo, err := collection.ByName(c.Params.ByName("name"))
	if err != nil {
		c.Fail(404, err)
		return
	}

	err = collection.LoadComplete(repo)
	if err != nil {
		c.Fail(500, err)
		return
	}

	resources := []string{string(repo.Key())}
	task, err:= runTaskInBackground(taskNamePrefix + repo.Name, resources, func(out *task.Output) error {
		fmt.Fprintln(out, "Loading packages...")
		list, err := deb.NewPackageListFromRefList(repo.RefList(), collectionFactory.PackageCollection(), nil)
		if err != nil {
			return err
		}

		// verify package refs and build package list
		for _, ref := range b.PackageRefs {
			var p *deb.Package

			p, err = collectionFactory.PackageCollection().ByKey([]byte(ref))
			if err != nil {
				if err == database.ErrNotFound {
					return fmt.Errorf("packages %s: %s", ref, err)
				}

				return err
			}
			err = cb(list, p, out)
			if err != nil {
				return err
			}
		}

		repo.UpdateRefList(deb.NewPackageRefListFromPackageList(list))

		return collectionFactory.LocalRepoCollection().Update(repo)
	})

	if err != nil {
		c.Fail(400, err)
		return
	}

	c.JSON(202, task)
}

// POST /repos/:name/packages
func apiReposPackagesAdd(c *gin.Context) {
	apiReposPackagesAddDelete(c, "Add packages to repo ", func(list *deb.PackageList, p *deb.Package, out *task.Output) error {
		fmt.Fprintf(out, "Adding package %s", p.Name)
		return list.Add(p)
	})
}

// DELETE /repos/:name/packages
func apiReposPackagesDelete(c *gin.Context) {
	apiReposPackagesAddDelete(c, "Delete packages from repo ", func(list *deb.PackageList, p *deb.Package, out *task.Output) error {
		fmt.Fprintf(out, "Removing package %s", p.Name)
		list.Remove(p)
		return nil
	})
}

// POST /repos/:name/file/:dir/:file
func apiReposPackageFromFile(c *gin.Context) {
	// redirect all work to dir method
	apiReposPackageFromDir(c)
}

// POST /repos/:name/file/:dir
func apiReposPackageFromDir(c *gin.Context) {
	forceReplace := c.Request.URL.Query().Get("forceReplace") == "1"
	noRemove := c.Request.URL.Query().Get("noRemove") == "1"

	if !verifyDir(c) {
		return
	}

	dirParam := c.Params.ByName("dir")
	fileParam := c.Params.ByName("file")
	if fileParam != "" && !verifyPath(fileParam) {
		c.Fail(400, fmt.Errorf("wrong file"))
		return
	}

	collectionFactory := context.NewCollectionFactory()
	collection := collectionFactory.LocalRepoCollection()

	name := c.Params.ByName("name")
	repo, err := collection.ByName(name)
	if err != nil {
		c.Fail(404, err)
		return
	}

	err = collection.LoadComplete(repo)
	if err != nil {
		c.Fail(500, err)
		return
	}

	taskName := fmt.Sprintf("Add packages from dir %s to repo %s", dirParam, name)
	if fileParam != "" {
		taskName = fmt.Sprintf("Add package %s from dir %s to repo %s", fileParam, dirParam, name)
	}

	resources := []string{string(repo.Key())}
	task, err := runTaskInBackground(taskName, resources, func(out *task.Output) error {
		verifier := &utils.GpgVerifier{}

		var (
			sources                      []string
			packageFiles, failedFiles    []string
			processedFiles, failedFiles2 []string
			reporter                     = &aptly.RecordingResultReporter{
				Warnings:     []string{},
				AddedLines:   []string{},
				RemovedLines: []string{},
			}
			list *deb.PackageList
		)

		if fileParam == "" {
			sources = []string{filepath.Join(context.UploadPath(), dirParam)}
		} else {
			sources = []string{filepath.Join(context.UploadPath(), dirParam, fileParam)}
		}

		packageFiles, failedFiles = deb.CollectPackageFiles(sources, reporter)

		list, err = deb.NewPackageListFromRefList(repo.RefList(), collectionFactory.PackageCollection(), nil)
		if err != nil {
			return fmt.Errorf("unable to load packages: %s", err)
		}

		processedFiles, failedFiles2, err = deb.ImportPackageFiles(list, packageFiles, forceReplace, verifier, context.PackagePool(),
			collectionFactory.PackageCollection(), reporter, nil)
		failedFiles = append(failedFiles, failedFiles2...)

		if err != nil {
			return fmt.Errorf("unable to import package files: %s", err)
		}

		repo.UpdateRefList(deb.NewPackageRefListFromPackageList(list))

		err = collectionFactory.LocalRepoCollection().Update(repo)
		if err != nil {
			return fmt.Errorf("unable to save: %s", err)
		}

		if !noRemove {
			processedFiles = utils.StrSliceDeduplicate(processedFiles)

			for _, file := range processedFiles {
				err := os.Remove(file)
				if err != nil {
					reporter.Warning("unable to remove file %s: %s", file, err)
				}
			}

			// atempt to remove dir, if it fails, that's fine: probably it's not empty
			os.Remove(filepath.Join(context.UploadPath(), dirParam))
		}

		if failedFiles == nil {
			failedFiles = []string{}
		}

		if len(reporter.AddedLines) > 0 {
			fmt.Fprintf(out, "Added: %s", strings.Join(reporter.AddedLines, ", "))
		}
		if len(reporter.RemovedLines) > 0 {
			fmt.Fprintf(out, "Removed: %s", strings.Join(reporter.RemovedLines, ", "))
		}
		if len(reporter.Warnings) > 0 {
			fmt.Fprintf(out, "Warnings: %s", strings.Join(reporter.Warnings, ", "))
		}
		if len(failedFiles) > 0 {
			fmt.Fprintf(out, "Failed files: %s", strings.Join(failedFiles, ", "))
		}

		return nil
	})

	if err != nil {
		c.Fail(400, err)
		return
	}

	c.JSON(202, task)
}
