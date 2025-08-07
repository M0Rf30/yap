# Enhanced Parser Example

This example demonstrates the improved PKGBUILD parser with support for custom variables and arrays in build() and package() functions.

## Features Added

1. **Two-Pass Parsing**: Variables are collected in the first pass and made available for function expansion
2. **Custom Variable Resolution**: Variables defined in PKGBUILD are available in build() and package() functions
3. **Array Support**: Full support for bash arrays and loops in functions
4. **Backward Compatibility**: All existing functionality preserved
5. **Hierarchical Variable Resolution**: PKGBUILD vars → Built-in vars → Environment vars

## Basic Custom Variables Example

```bash
pkgname=myapp
pkgver=2.1.0
pkgrel=1
pkgdesc="My awesome application"
arch=('x86_64')
license=('MIT')

# Custom variables that can be used in functions
install_prefix="/opt/mycompany"
binary_name="${pkgname}-${pkgver}"
config_dir="/etc/${pkgname}"
service_user="myapp"

build() {
    cd "${srcdir}/${pkgname}"
    
    # Custom variables are now expanded properly
    echo "Building ${binary_name} for installation in ${install_prefix}"
    
    make DESTDIR="${install_prefix}" \
         CONFIG_DIR="${config_dir}" \
         SERVICE_USER="${service_user}"
}

package() {
    cd "${srcdir}/${pkgname}"
    
    # Create directories using custom variables
    mkdir -p "${pkgdir}${install_prefix}/bin"
    mkdir -p "${pkgdir}${config_dir}"
    mkdir -p "${pkgdir}/usr/lib/systemd/system"
    
    # Install binary with custom name
    install -Dm755 "${binary_name}" "${pkgdir}${install_prefix}/bin/${pkgname}"
    
    # Install config with custom path
    install -Dm644 "${pkgname}.conf" "${pkgdir}${config_dir}/${pkgname}.conf"
    
    # Generate systemd service with custom variables
    cat > "${pkgdir}/usr/lib/systemd/system/${pkgname}.service" << EOF
[Unit]
Description=My App Service

[Service]
ExecStart=${install_prefix}/bin/${pkgname}
User=${service_user}
Group=${service_user}

[Install]
WantedBy=multi-user.target
EOF
}
```

## Array Support Example

The enhanced parser also supports bash arrays and loops:

```bash
pkgname=myapp
pkgver=2.1.0
pkgrel=1
pkgdesc="My awesome application with multiple components"
arch=('x86_64')
license=('MIT')

# Arrays for organizing installation components
binary_names=("myapp" "myapp-cli" "myapp-daemon" "myapp-worker")
config_files=("app.conf" "database.conf" "logging.conf" "cache.conf")
service_files=("myapp.service" "myapp-worker.service")
doc_files=("README.md" "INSTALL.md" "CONFIG.md" "API.md")

# Custom variables combined with arrays
install_prefix="/opt/mycompany"
config_dir="/etc/${pkgname}"

build() {
    cd "${srcdir}/${pkgname}-${pkgver}"
    
    # Build all binaries using array loop
    for binary in "${binary_names[@]}"; do
        echo "Building ${binary}..."
        make "${binary}" DESTDIR="${install_prefix}"
    done
}

package() {
    cd "${srcdir}/${pkgname}-${pkgver}"
    
    # Install all binaries
    for binary in "${binary_names[@]}"; do
        install -Dm755 "${binary}" "${pkgdir}${install_prefix}/bin/${binary}"
    done
    
    # Install configuration files
    mkdir -p "${pkgdir}${config_dir}"
    for config in "${config_files[@]}"; do
        install -Dm644 "config/${config}" "${pkgdir}${config_dir}/${config}"
    done
    
    # Install systemd services
    for service in "${service_files[@]}"; do
        install -Dm644 "systemd/${service}" "${pkgdir}/usr/lib/systemd/system/${service}"
    done
    
    # Install documentation
    mkdir -p "${pkgdir}/usr/share/doc/${pkgname}"
    for doc in "${doc_files[@]}"; do
        install -Dm644 "docs/${doc}" "${pkgdir}/usr/share/doc/${pkgname}/${doc}"
    done
    
    # Create necessary runtime directories
    mkdir -p "${pkgdir}/var/lib/${pkgname}"
    mkdir -p "${pkgdir}/var/log/${pkgname}"
}
```

## Technical Implementation

The enhanced parser:

1. **Collects Variables**: First pass identifies all variable assignments (including arrays)
2. **Custom Environment Function**: Creates a resolver that checks:
   - Custom PKGBUILD variables first (including array expansions)
   - Built-in variables (pkgdir, srcdir, etc.)
   - Environment variables as fallback
3. **Function Expansion**: Second pass expands function bodies using mvdan/sh with custom resolver
4. **Array Support**: Bash arrays expand to space-separated values in loops and commands
5. **Error Handling**: Graceful fallback if expansion fails

## Variable Expansion Behavior

- **${variable}**: PKGBUILD variables get expanded during parsing
- **$variable**: Shell runtime variables are removed during parsing (correct behavior)
- **Arrays**: Expand to space-separated strings (e.g., `${files[@]}` → `file1.txt file2.txt file3.txt`)

## Benefits

- **More Flexible PKGBUILDs**: Define reusable variables and arrays for paths, names, versions
- **Cleaner Code**: Reduce duplication in build() and package() functions with arrays and loops
- **Better Maintainability**: Change paths/names/file lists in one place
- **Dynamic Content**: Generate configuration files with variable substitution  
- **Organized Installation**: Group related files in arrays for easier management

## Advanced Features

- **Array Loops**: `for file in "${files[@]}"; do ... done`
- **Array Length**: `${#array[@]}`
- **Array Indexing**: `${array[0]}`, `${array[-1]}`
- **Parameter Expansion**: `${files%% *}` (first element)
- **Mixed Variables**: Combine arrays with scalar variables

## Backward Compatibility

- All existing PKGBUILDs continue to work unchanged
- Built-in variables (${pkgdir}, ${srcdir}, etc.) still function normally
- Environment variable expansion preserved
- No performance impact for PKGBUILDs without custom variables
- Array support is entirely optional - only activated when arrays are defined

## Testing

Comprehensive tests verify both basic variable expansion and array support:

```bash
# Run all parser tests
go test -v ./pkg/parser

# Run just array tests  
go test -v ./pkg/parser -run "TestArray"
```

## Documentation

- See `ARRAY_SUPPORT.md` for detailed array functionality documentation
- All features are backward compatible with existing PKGBUILD files