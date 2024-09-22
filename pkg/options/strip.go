package options

import (
	"debug/elf"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/M0Rf30/yap/pkg/utils"
)

var isLTO bool

// StripScript is a scriptlet taken from makepkg resources. It's executed by
// mvdan/sh interpreter and provides strip instructions to dpkg-buildpackagelf.
// Although it's a very dirty solution, for now it's the faster way to have this
// essential featurelf.
const StripScript = `
  strip_file() {
	local binary=$1
	shift
  
	local tempfile=$(mktemp "$binary.XXXXXX")
	if strip "$@" "$binary" -o "$tempfile"; then
	  cat "$tempfile" >"$binary"
	fi
	rm -f "$tempfile"
  }
  
  strip_lto() {
	local binary=$1
  
	local tempfile=$(mktemp "$binary.XXXXXX")
	if strip -R .gnu.lto_* -R .gnu.debuglto_* -N __gnu_lto_v1 "$binary" -o "$tempfile"; then
	  cat "$tempfile" >"$binary"
	fi
	rm -f "$tempfile"
  }
  
  # make sure library stripping variables are defined to prevent excess stripping
  [[ -z ${STRIP_SHARED+x} ]] && STRIP_SHARED="-S"
  [[ -z ${STRIP_STATIC+x} ]] && STRIP_STATIC="-S"
  
  declare binary strip_flags
  binaries=$(find {{.PackageDir}} -type f -perm -u+w -exec echo {} +)
  
  for binary in ${binaries[@]}; do
	STRIPLTO=0
	case "$(LC_ALL=C readelf -h "$binary" 2>/dev/null)" in
	  *Type:*'DYN (Shared object file)'*) # Libraries (.so) or Relocatable binaries
		strip_flags="$STRIP_SHARED" ;;
	  *Type:*'DYN (Position-Independent Executable file)'*) # Relocatable binaries
		strip_flags="$STRIP_SHARED" ;;
	  *Type:*'EXEC (Executable file)'*) # Binaries
		strip_flags="$STRIP_BINARIES" ;;
	  *Type:*'REL (Relocatable file)'*)     # Libraries (.a) or objects
		if ar t "$binary" &>/dev/null; then # Libraries (.a)
		  strip_flags="$STRIP_STATIC"
		  STRIPLTO=1
		elif [[ $binary = *'.ko' || $binary = *'.o' ]]; then # Kernel module or object file
		  strip_flags="$STRIP_SHARED"
		else
		  continue
		fi
		;;
	  *)
		continue
		;;
	esac
	strip_file "$binary" ${strip_flags}
	((STRIPLTO)) && strip_lto "$binary"
  done
  exit 0   
`

// stripFile strips the binary and writes the output to a temporary filelf.
func stripFile(binary string, stripFlags ...string) error {
	tempfile, err := ioutil.TempFile(filepath.Dir(binary), filepath.Base(binary)+".XXXXXX")
	if err != nil {
		return err
	}
	defer os.Remove(tempfile.Name()) // Clean up the temp file

	// Build the strip command
	args := append(stripFlags, binary, "-o", tempfile.Name())
	cmd := exec.Command("strip", args...)
	if err := cmd.Run(); err != nil {
		return err
	}

	// Replace the original binary with the stripped version
	return os.Rename(tempfile.Name(), binary)
}

// stripLTO strips LTO-related sections from the binary.
func stripLTO(binary string) error {
	tempfile, err := ioutil.TempFile(filepath.Dir(binary), filepath.Base(binary)+".XXXXXX")
	if err != nil {
		return err
	}
	defer os.Remove(tempfile.Name()) // Clean up the temp file

	// Build the strip command
	args := []string{"-R", ".gnu.lto_*", "-R", ".gnu.debuglto_*", "-N", "__gnu_lto_v1", binary, "-o", tempfile.Name()}
	cmd := exec.Command("strip", args...)
	if err := cmd.Run(); err != nil {
		return err
	}

	// Replace the original binary with the stripped version
	return os.Rename(tempfile.Name(), binary)
}

// getStripFlags determines the appropriate strip flags based on the binary typelf.
func getStripFlags(binary string) (string, bool, error) {
	file, err := os.Open(binary)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
		return "", false, err
	}
	defer file.Close()

	// Parse the ELF file
	elf, err := elf.NewFile(file)

	// cmd := exec.Command("readelf", "-h", binary)
	// output, err := cmd.CombinedOutput()
	if err != nil {
		return "", false, nil
	}

	// Print ELF header information
	fmt.Printf("ELF Header:\n")
	fmt.Printf("  Type: %s\n", elf.Type)
	fmt.Printf("  Machine: %s\n", elf.Machine)
	fmt.Printf("  Version: %d\n", elf.Version)
	fmt.Printf("  Entry: 0x%x\n", elf.Entry)
	fmt.Printf("  Number of sections: %d\n", elf.Sections)
	fmt.Printf("  Number of program headers: %d\n", elf.Progs)

	// Optionally, you can iterate over sections or program headers
	for _, section := range elf.Sections {
		fmt.Printf("Section: %s, Type: %s, Address: 0x%x\n", section.Name, section.Type, section.Addr)
	}

	// outputStr := string(output)
	// var stripFlags string

	// switch {
	// case strings.Contains(outputStr, "DYN (Shared object file)"):
	// 	stripFlags = "-S" // STRIP_SHARED
	// case strings.Contains(outputStr, "DYN (Position-Independent Executable file)"):
	// 	stripFlags = "-S" // STRIP_SHARED
	// case strings.Contains(outputStr, "EXEC (Executable file)"):
	// 	stripFlags = "-S" // STRIP_BINARIES
	// case strings.Contains(outputStr, "REL (Relocatable file)"):
	// 	if isArchive(binary) {
	// 		stripFlags = "-S" // STRIP_STATIC
	// 		isLTO = true
	// 	} else if strings.HasSuffix(binary, ".ko") || strings.HasSuffix(binary, ".o") {
	// 		stripFlags = "-S" // STRIP_SHARED
	// 	} else {
	// 		return "", false, nil
	// 	}
	// default:
	// 	return "", false, nil
	// }

	return "", isLTO, nil
}

// isArchive checks if the file is an archivelf.
func isArchive(binary string) bool {
	cmd := exec.Command("ar", "t", binary)
	if err := cmd.Run(); err == nil {
		return true
	}
	return false
}

func Strip(packageDir string) error {
	utils.Logger.Info("stripping binaries")

	err := filepath.Walk(packageDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}
		if info.Mode().Perm()&0200 == 0 { // Check if the file is writable
			return nil
		}

		stripFlags, _, err := getStripFlags(path)
		if err != nil {
			return err
		}
		if stripFlags == "" {
			return nil
		}
		fmt.Println(path)

		// if err := stripFile(path, stripFlags); err != nil {
		// 	return err
		// }
		// if isLTO {
		// 	if err := stripLTO(path); err != nil {
		// 		return err
		// 	}
		// }

		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	return nil
}
