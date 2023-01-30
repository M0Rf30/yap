package pacman

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/M0Rf30/yap/pack"
	"github.com/M0Rf30/yap/utils"
)

type Pacman struct {
	Pack      *pack.Pack
	pacmanDir string
}

func (p *Pacman) getDepends() error {
	var err error
	if len(p.Pack.MakeDepends) == 0 {
		return err
	}

	args := []string{
		"-S",
		"--noconfirm",
	}
	args = append(args, p.Pack.MakeDepends...)

	err = utils.Exec("", "pacman", args...)
	if err != nil {
		return err
	}

	return err
}

func (p *Pacman) getUpdates() error {
	err := utils.Exec("", "sudo", "pacman", "-Sy")
	if err != nil {
		return err
	}

	return err
}

func (p *Pacman) createInstall() error {
	path := filepath.Join(p.pacmanDir, p.Pack.PkgName+".install")

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

	if err != nil {
		log.Fatal(err)
	}

	if pack.Verbose {
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

	if err != nil {
		log.Fatal(err)
	}

	if pack.Verbose {
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

func (p *Pacman) Prep() error {
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
	p.pacmanDir = filepath.Join(p.Pack.Root, "pacman")

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
		if err := utils.Exec("", "sudo", "pacman", "-U", "--noconfirm", pkg); err != nil {
			return err
		}
	}

	return nil
}
