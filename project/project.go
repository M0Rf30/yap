package project

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/builder"
	"github.com/M0Rf30/yap/constants"
	"github.com/M0Rf30/yap/packer"
	"github.com/M0Rf30/yap/parser"
	"github.com/M0Rf30/yap/utils"
)

var SkipSyncFlag bool

type DistroProject interface {
	Create() error
	Prep() error
}

type Project struct {
	Builder        *builder.Builder
	BuildRoot      string
	DependsOn      []*Project
	Distro         string
	MirrorRoot     string
	PackageManager packer.Packer
	Path           string
	Release        string
	Root           string
	Name           string `json:"name"`
	HasToInstall   bool   `json:"install"`
}

type MultipleProject struct {
	packageManager packer.Packer
	root           string
	BuildDir       string     `json:"buildDir"`
	Description    string     `json:"description"`
	Name           string     `json:"name"`
	Output         string     `json:"output"`
	Projects       []*Project `json:"projects"`
}

func (mpc *MultipleProject) BuildAll() error {
	for _, proj := range mpc.Projects {
		fmt.Printf("%s🚀 :: %s%s: launching build for project ...%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			proj.Name,
			string(constants.ColorWhite))

		if err := proj.Builder.Build(); err != nil {
			return err
		}

		artefactPaths, err := proj.PackageManager.Build()
		if err != nil {
			return err
		}

		if mpc.Output != "" {
			if err := utils.ExistsMakeDir(mpc.Output); err != nil {
				return err
			}

			for _, ap := range artefactPaths {
				filename := filepath.Base(ap)
				if err := utils.Copy("", ap, filepath.Join(mpc.Output, filename), false); err != nil {
					return err
				}
			}
		}

		if proj.HasToInstall {
			fmt.Printf("%s🤓 :: %s%s: installing package ...%s\n",
				string(constants.ColorBlue),
				string(constants.ColorYellow),
				proj.Name,
				string(constants.ColorWhite))

			if err := proj.PackageManager.Install(); err != nil {
				return err
			}
		}
	}

	return nil
}

func (mpc *MultipleProject) Clean(cleanFlag bool) error {
	var err error

	if cleanFlag {
		for _, project := range mpc.Projects {
			err = utils.RemoveAll(project.Builder.PKGBUILD.SourceDir)
			if err != nil {
				return err
			}
		}
	}

	for _, project := range mpc.Projects {
		err = utils.RemoveAll(project.Builder.PKGBUILD.PackageDir)
		if err != nil {
			return err
		}
	}

	return err
}

func (mpc *MultipleProject) MultiProject(distro string, release string, path string) error {
	err := mpc.readProject(path)
	if err != nil {
		return err
	}

	err = mpc.getPackageManger(distro, release, path)
	if err != nil {
		return err
	}

	err = utils.ExistsMakeDir(mpc.BuildDir)
	if err != nil {
		return err
	}

	err = mpc.validateAllProject(distro, release, path)
	if err != nil {
		return err
	}

	if !SkipSyncFlag {
		if err := mpc.packageManager.Update(); err != nil {
			return err
		}
	}

	err = mpc.populateProjects(distro, release, path)
	if err != nil {
		return err
	}

	mpc.root = path

	return err
}

func (mpc *MultipleProject) getPackageManger(distro string, release string, path string) error {
	pkgbuild, err := parser.ParseFile(distro, release,
		filepath.Join(mpc.BuildDir, mpc.Projects[0].Name),
		filepath.Join(path, mpc.Projects[0].Name))

	if err != nil {
		return err
	}

	mpc.packageManager = packer.GetPackageManager(pkgbuild, distro, release)

	return err
}

func (mpc *MultipleProject) populateProjects(distro string, release string, path string) error {
	var err error

	var projects = make([]*Project, 0)

	for _, child := range mpc.Projects {
		pkgbuild, err := parser.ParseFile(distro, release, filepath.Join(mpc.BuildDir, child.Name),
			filepath.Join(path, child.Name))
		if err != nil {
			return err
		}

		if err != nil {
			return err
		}

		if err = mpc.packageManager.Prep(); err != nil {
			return err
		}

		proj := &Project{
			Name:           child.Name,
			DependsOn:      nil,
			Builder:        &builder.Builder{PKGBUILD: pkgbuild},
			PackageManager: mpc.packageManager,
			HasToInstall:   child.HasToInstall,
		}

		projects = append(projects, proj)
	}

	mpc.Projects = projects

	return err
}

func (mpc *MultipleProject) readProject(path string) error {
	file, err := os.Open(filepath.Join(path, "yap.json"))
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to open yap.json file within '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			path,
			string(constants.ColorWhite))
		os.Exit(1)
	}

	prjContent, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	err = json.Unmarshal(prjContent, &mpc)

	return err
}

func (mpc *MultipleProject) validateAllProject(distro string, release string, path string) error {
	var err error
	for _, child := range mpc.Projects {
		pkgbuild, err := parser.ParseFile(distro, release, filepath.Join(mpc.BuildDir, child.Name), filepath.Join(path, child.Name))
		if err != nil {
			return err
		}

		pkgbuild.Validate()
	}

	return err
}
