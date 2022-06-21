package parse

import (
	"bufio"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/packagefoundation/yap/pack"
	"github.com/packagefoundation/yap/utils"
)

const (
	blockList = 1
	blockFunc = 2
)

var itemReg = regexp.MustCompile("(\"[^\"]+\")|(`[^`]+`)")

func File(distro, release, compiledOutput, home string) (*pack.Pack, error) { // nolint:gocognit
	home, err := filepath.Abs(home)

	path := filepath.Join(compiledOutput, "PKGBUILD")

	pac := &pack.Pack{
		Distro:     distro,
		Release:    release,
		Root:       compiledOutput,
		Home:       home,
		SourceDir:  filepath.Join(compiledOutput, "src"),
		PackageDir: filepath.Join(compiledOutput, "pkg"),
	}

	if err != nil {
		fmt.Printf("parse: Failed to get root directory from '%s'\n",
			home)

		return pac, err
	}

	err = utils.ExistsMakeDir(compiledOutput)
	if err != nil {
		return pac, err
	}

	err = utils.CopyFiles(home, compiledOutput, false)
	if err != nil {
		return pac, err
	}

	pac.Init()

	file, err := utils.Open(path)
	if err != nil {
		return pac, err
	}
	defer file.Close()

	num := 0
	blockType := 0
	blockKey := ""
	blockData := ""
	blockItems := []string{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		num++

		if line == "" || line[:1] == "#" {
			continue
		}

		if blockType == blockList { //nolint:gocritic,nestif
			if line == ")" {
				for _, item := range itemReg.FindAllString(blockData, -1) {
					blockItems = append(blockItems, item[1:len(item)-1])
				}

				err = pac.AddItem(blockKey, blockItems, num, line)

				if err != nil {
					return pac, err
				}

				blockType = 0
				blockKey = ""
				blockData = ""
				blockItems = []string{}

				continue
			}

			blockData += strings.TrimSpace(line)
		} else if blockType == blockFunc {
			if line == "}" {
				err = pac.AddItem(blockKey, blockItems, num, line)
				if err != nil {
					return pac, err
				}
				blockType = 0
				blockKey = ""
				blockItems = []string{}

				continue
			}

			blockItems = append(blockItems, strings.TrimSpace(line))
		} else {
			if strings.Contains(line, "() {") {
				blockType = blockFunc
				blockKey = strings.Split(line, "() {")[0]
			} else {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) != 2 {
					fmt.Printf("parse: Line missing '=' (%d: %s)",
						num, line)

					return pac, err
				}

				key := parts[0]
				val := parts[1]

				if key[:1] == " " {
					fmt.Printf("parse: Extra space padding (%d: %s)",
						num, line)

					return pac, err
				} else if key[len(key)-1:] == " " {
					fmt.Printf(
						"parse: Extra space before '=' (%d: %s)",
						num, line)

					return pac, err
				}

				valLen := len(val)
				switch val[:1] {
				case `"`, "`":
					if val[valLen-1:] != val[:1] {
						fmt.Printf("parse: Unexpected char '%s' "+
							"expected '%s' (%d: %s)",
							val[valLen-1:], val[:1], num, line)

						return pac, err
					}

					err = pac.AddItem(key, val[1:valLen-1], num, line)
					if err != nil {
						return pac, err
					}
				case "(":
					if val[valLen-1:] == ")" {
						if val[1:2] != `"` && val[1:2] != "`" {
							fmt.Printf("parse: Unexpected char '%s' "+
								"expected '\"' or '`' (%d: %s)",
								val[1:2], num, line)

							return pac, err
						}

						if val[valLen-2:valLen-1] != val[1:2] {
							fmt.Printf("parse: Unexpected char '%s' "+
								"expected '%s' (%d: %s)",
								val[valLen-2:valLen-1], val[1:2],
								num, line)

							return pac, err
						}

						val = val[2 : len(val)-2]
						err = pac.AddItem(key, []string{val}, num, line)
						if err != nil {
							return pac, err
						}
					} else {
						blockType = blockList
						blockKey = key
					}
				case " ":
					fmt.Printf(
						"parse: Extra space after '=' (%d: %s)",
						num, line)

					return pac, err
				default:
					fmt.Printf(
						"parse: Unexpected char '%s' expected "+
							"'\"' or '`' (%d: %s)", val[:1], num, line)

					return pac, err
				}
			}
		}
	}

	return pac, err
}
