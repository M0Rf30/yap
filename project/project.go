package project

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/packagefoundation/yap/builder"
	"github.com/packagefoundation/yap/constants"
	"github.com/packagefoundation/yap/packer"
	"github.com/packagefoundation/yap/parse"
	"github.com/packagefoundation/yap/utils"
)

type DistroProject interface {
	Prep() error
	Create() error
}

type singleProjectConf struct {
	Name    string `json:"name"`
	Install bool   `json:"install"`
	// TODO we could easily build a dep tree and then build all the stuff in parallel automatically
	//Parallel bool `json:"parallel"`
	//DependsOn []string `json:"depends_on"`
}

type multipleProjectConf struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Output      string              `json:"output"`
	BuildDir    string              `json:"build_dir"`
	Projects    []singleProjectConf `json:"projects"`
}

type Project struct {
	Name         string
	DependsOn    []*Project
	Builder      *builder.Builder
	Packer       packer.Packer
	HasToInstall bool
}

type MultipleProject struct {
	project  []*Project
	root     string
	output   string
	buildDir string
}

func (m *MultipleProject) NoCache() error {
	return os.RemoveAll(m.buildDir)
}

func (m *MultipleProject) Close() error {
	for _, p := range m.project {
		os.RemoveAll(p.Builder.Pack.PackageDir)
	}
	return nil
}

func NewMultipleProject(distro string, release string, path string) (*MultipleProject, error) {
	file, err := os.Open(filepath.Join(path, "yap.json"))

	if err != nil {
		file, err = os.Open(filepath.Join(path, "pacur.json"))
		if err != nil {
			fmt.Printf("%s❌ :: %sfailed to open yap.json (pacur.json) file within '%s'%s\n", string(constants.ColorBlue), string(constants.ColorYellow), path, string(constants.ColorWhite))
			os.Exit(1)
		}
	}

	prjBsContent, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	mpc := multipleProjectConf{}
	if err := json.Unmarshal(prjBsContent, &mpc); err != nil {
		return nil, err
	}

	projects := make([]*Project, 0)
	buildDir := filepath.Join(os.TempDir())
	if mpc.BuildDir != "" {
		buildDir = filepath.Join(mpc.BuildDir)
	}
	if err := utils.ExistsMakeDir(buildDir); err != nil {
		return nil, err
	}
	for _, child := range mpc.Projects {
		pac, err := parse.File(distro, release, filepath.Join(buildDir, child.Name), filepath.Join(path, child.Name))
		if err != nil {
			return nil, err
		}
		if err := pac.Compile(); err != nil {
			return nil, err
		}
		pcker, err := packer.GetPacker(pac, distro, release)
		if err != nil {
			return nil, err
		}
		if err := pcker.Prep(); err != nil {
			return nil, err
		}

		p := &Project{
			Name:         child.Name,
			DependsOn:    nil,
			Builder:      &builder.Builder{Pack: pac},
			Packer:       pcker,
			HasToInstall: child.Install,
		}

		projects = append(projects, p)
	}

	return &MultipleProject{
		project:  projects,
		root:     path,
		output:   mpc.Output,
		buildDir: buildDir,
	}, nil
}

func (m *MultipleProject) BuildAll() error {
	for _, p := range m.project {
		fmt.Printf("%s🚀 :: %s%s: launching build for project ...%s\n", string(constants.ColorBlue), string(constants.ColorYellow), p.Name, string(constants.ColorWhite))
		if err := p.Builder.Build(); err != nil {
			return err
		}
		artefactPaths, err := p.Packer.Build()
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
		if p.HasToInstall {
			fmt.Printf("%s🤓 :: %s%s: installing package ...%s\n", string(constants.ColorBlue), string(constants.ColorYellow), p.Name, string(constants.ColorWhite))
			if err := p.Packer.Install(); err != nil {
				return err
			}
		}
	}
	return nil
}
