package options

// StripScript is a scriptlet taken from makepkg resources. It's executed by
// mvdan/sh interpreter and provides strip instructions to dpkg-buildpackage.
// Although it's a very dirty solution, for now it's the faster way to have this
// essential feature.
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
  [[ -z ${STRIP_SHARED+x} ]] && STRIP_SHARED="-s"
  [[ -z ${STRIP_STATIC+x} ]] && STRIP_STATIC="-s"
  
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
