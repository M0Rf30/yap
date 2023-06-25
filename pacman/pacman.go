package pacman

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/M0Rf30/yap/utils"
	"github.com/otiai10/copy"
)

type Pacman struct {
	PKGBUILD  *pkgbuild.PKGBUILD
	pacmanDir string
}

func (p *Pacman) getDepends() error {
	var err error
	if len(p.PKGBUILD.MakeDepends) == 0 {
		return err
	}

	args := []string{
		"-S",
		"--noconfirm",
	}
	args = append(args, p.PKGBUILD.MakeDepends...)

	err = utils.Exec("", "pacman", args...)
	if err != nil {
		return err
	}

	return err
}

func (p *Pacman) getUpdates() error {
	err := utils.Exec("", "pacman", "-Sy")
	if err != nil {
		return err
	}

	return err
}

func (p *Pacman) createInstall() error {
	path := filepath.Join(p.pacmanDir, p.PKGBUILD.PkgName+".install")

	file, err := os.Create(path)

	if err != nil {
		log.Fatal(err)
	}

	// remember to close the file
	defer file.Close()

	// create new buffer
	writer := io.Writer(file)

	tmpl := template.New(".install")
	tmpl.Funcs(template.FuncMap{
		"join": func(strs []string) string {
			return strings.Trim(strings.Join(strs, ", "), " ")
		},
		"multiline": func(strs string) string {
			ret := strings.ReplaceAll(strs, "\n", "\n ")

			return strings.Trim(ret, " \n")
		},
	})

	template.Must(tmpl.Parse(postInstall))

	if pkgbuild.Verbose {
		err = tmpl.Execute(os.Stdout, p)
		if err != nil {
			log.Fatal(err)
		}
	}

	err = tmpl.Execute(writer, p)
	if err != nil {
		log.Fatal(err)
	}

	return err
}

func (p *Pacman) createMake() error {
	path := filepath.Join(p.pacmanDir, "PKGBUILD")
	file, err := os.Create(path)

	if err != nil {
		log.Fatal(err)
	}

	// remember to close the file
	defer file.Close()

	// create new buffer
	writer := io.Writer(file)

	tmpl := template.New("pkgbuild")
	tmpl.Funcs(template.FuncMap{
		"join": func(strs []string) string {
			return strings.Trim(strings.Join(strs, " "), "\n")
		},
		"multiline": func(strs string) string {
			ret := strings.ReplaceAll(strs, "\n", "\n ")

			return strings.Trim(ret, " \n")
		},
	})

	template.Must(tmpl.Parse(specFile))

	if pkgbuild.Verbose {
		err = tmpl.Execute(os.Stdout, p)
		if err != nil {
			log.Fatal(err)
		}
	}

	err = tmpl.Execute(writer, p)
	if err != nil {
		log.Fatal(err)
	}

	return err
}

func (p *Pacman) pacmanBuild() error {
	err := utils.Exec(p.pacmanDir, "makepkg", "-f")
	if err != nil {
		return err
	}

	return err
}

func (p *Pacman) Prepare() error {
	err := p.getDepends()
	if err != nil {
		return err
	}

	return err
}

func (p *Pacman) Update() error {
	err := p.getUpdates()
	if err != nil {
		return err
	}

	return err
}

func (p *Pacman) makePackerDir() error {
	err := utils.ExistsMakeDir(p.pacmanDir)
	if err != nil {
		return err
	}

	return err
}

func (p *Pacman) Build() ([]string, error) {
	p.pacmanDir = filepath.Join(p.PKGBUILD.StartDir)
	stagingDir := filepath.Join(p.pacmanDir, "staging", p.PKGBUILD.PkgName)

	err := utils.RemoveAll(p.pacmanDir)
	if err != nil {
		return nil, err
	}

	err = p.makePackerDir()
	if err != nil {
		return nil, err
	}

	err = p.createMake()
	if err != nil {
		return nil, err
	}

	err = p.createInstall()
	if err != nil {
		return nil, err
	}

	err = copy.Copy(p.PKGBUILD.PackageDir, stagingDir)
	if err != nil {
		return nil, err
	}

	err = p.pacmanBuild()
	if err != nil {
		return nil, err
	}

	pkgs, err := utils.FindExt(p.pacmanDir, ".zst")
	if err != nil {
		return nil, err
	}

	return pkgs, nil
}

func (p *Pacman) Install() error {
	pkgs, err := utils.FindExt(p.pacmanDir, ".zst")
	if err != nil {
		return err
	}

	for _, pkg := range pkgs {
		if err := utils.Exec("", "pacman", "-U", "--noconfirm", pkg); err != nil {
			return err
		}
	}

	return nil
}

func (p *Pacman) PrepareEnvironment(golang bool) error {
	var err error

	args := []string{
		"-S",
		"--noconfirm",
	}
	args = append(args, buildEnvironmentDeps...)

	if golang {
		utils.CheckGO()

		args = append(args, "go")
	}

	err = utils.Exec("", "pacman", args...)
	if err != nil {
		return err
	}

	return err
}
