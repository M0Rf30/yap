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
	"github.com/M0Rf30/yap/parse"
	"github.com/M0Rf30/yap/utils"
)

type DistroProject interface {
	Prep() error
	Create() error
}

type singleProjectConf struct {
	Name    string `json:"name"`
	Install bool   `json:"install"`
}

type multipleProjectConf struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Output      string              `json:"output"`
	BuildDir    string              `json:"buildDir"`
	Projects    []singleProjectConf `json:"projects"`
}

type Project struct {
	BuildRoot    string
	Builder      *builder.Builder
	DependsOn    []*Project
	Distro       string
	HasToInstall bool
	MirrorRoot   string
	Name         string
	Packer       packer.Packer
	Path         string
	Release      string
	Root         string
}

type MultipleProject struct {
	project  []*Project
	root     string
	output   string
	buildDir string
}

func (m *MultipleProject) Clean(cleanFlag bool) error {
	var err error

	if cleanFlag {
		for _, project := range m.project {
			err = utils.RemoveAll(project.Builder.Pack.SourceDir)
			if err != nil {
				return err
			}

			err = utils.RemoveAll(project.Builder.Pack.PackageDir)
			if err != nil {
				return err
			}
		}
	}

	for _, project := range m.project {
		err = utils.RemoveAll(project.Builder.Pack.PackageDir)
		if err != nil {
			return err
		}
	}

	return err
}

func MultiProject(distro string, release string, path string) (*MultipleProject, error) {
	file, err := os.Open(filepath.Join(path, "yap.json"))
	if err != nil {
		file, err = os.Open(filepath.Join(path, "pacur.json"))
		if err != nil {
			fmt.Printf("%s❌ :: %sfailed to open yap.json (pacur.json) file within '%s'%s\n",
				string(constants.ColorBlue),
				string(constants.ColorYellow),
				path,
				string(constants.ColorWhite))
			os.Exit(1)
		}
	}

	prjBsContent, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	mpc := multipleProjectConf{}

	if err := json.Unmarshal(prjBsContent, &mpc); err != nil {
		return nil, err
	}

	projects := make([]*Project, 0)
	buildDir := os.TempDir()

	if mpc.BuildDir != "" {
		buildDir = mpc.BuildDir
	}

	if err := utils.ExistsMakeDir(buildDir); err != nil {
		return nil, err
	}

	pac, err := parse.File(distro, release,
		filepath.Join(buildDir, mpc.Projects[0].Name),
		filepath.Join(path, mpc.Projects[0].Name))
	if err != nil {
		return nil, err
	}

	pcker := packer.GetPacker(pac, distro, release)

	for _, child := range mpc.Projects {
		pac, err := parse.File(distro, release, filepath.Join(buildDir, child.Name), filepath.Join(path, child.Name))
		if err != nil {
			return nil, err
		}

		pac.Validate()
	}

	if err := pcker.Update(); err != nil {
		return nil, err
	}

	for _, child := range mpc.Projects {
		pac, err := parse.File(distro, release, filepath.Join(buildDir, child.Name), filepath.Join(path, child.Name))
		if err != nil {
			return nil, err
		}

		pcker := packer.GetPacker(pac, distro, release)

		if err != nil {
			return nil, err
		}

		if err := pcker.Prep(); err != nil {
			return nil, err
		}

		proj := &Project{
			Name:         child.Name,
			DependsOn:    nil,
			Builder:      &builder.Builder{Pack: pac},
			Packer:       pcker,
			HasToInstall: child.Install,
		}

		projects = append(projects, proj)
	}

	return &MultipleProject{
		project:  projects,
		root:     path,
		output:   mpc.Output,
		buildDir: buildDir,
	}, nil
}

func (m *MultipleProject) BuildAll() error {
	for _, proj := range m.project {
		fmt.Printf("%s🚀 :: %s%s: launching build for project ...%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			proj.Name,
			string(constants.ColorWhite))

		if err := proj.Builder.Build(); err != nil {
			return err
		}

		artefactPaths, err := proj.Packer.Build()
		if err != nil {
			return err
		}

		if m.output != "" {
			if err := utils.ExistsMakeDir(m.output); err != nil {
				return err
			}

			for _, ap := range artefactPaths {
				filename := filepath.Base(ap)
				if err := utils.Copy("", ap, filepath.Join(m.output, filename), false); err != nil {
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

			if err := proj.Packer.Install(); err != nil {
				return err
			}
		}
	}

	return nil
}
