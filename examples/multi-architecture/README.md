# Multi-Architecture PKGBUILD Example

This example demonstrates YAP's new architecture-specific variable support, following the Arch Linux PKGBUILD convention with single underscore (`_`) syntax.

## Features Demonstrated

### 1. Architecture-Specific Dependencies
```bash
# Base dependencies
depends=('glibc')

# Architecture-specific overrides (Priority 4 - highest)
depends_x86_64=('glibc' 'lib32-glibc')
depends_aarch64=('glibc' 'aarch64-linux-gnu-binutils')
depends_armv7h=('glibc' 'arm-linux-gnueabihf-binutils')
```

### 2. Architecture-Specific Sources
```bash
# Generic source
source=("https://example.com/generic-${pkgver}.tar.gz")

# Architecture-optimized sources
source_x86_64=("https://example.com/x86_64-optimized-${pkgver}.tar.gz")
source_aarch64=("https://example.com/aarch64-${pkgver}.tar.gz")
```

### 3. Priority System
The complete priority system (highest to lowest):

1. **Priority 4** - Architecture-specific: `variable_x86_64`
2. **Priority 3** - Full distribution: `variable__ubuntu_noble`
3. **Priority 2** - Distribution: `variable__ubuntu`
4. **Priority 1** - Package manager: `variable__apt`
5. **Priority 0** - Base variable: `variable`

### 4. Combined Usage
```bash
# This demonstrates priority override
depends__ubuntu_noble=('libc6-dev' 'ubuntu-noble-specific')  # Priority 3
depends_x86_64=('glibc' 'lib32-glibc' 'x86_64-optimized-lib') # Priority 4 - wins!

# Result on x86_64 + Ubuntu Noble: uses the x86_64-specific dependencies
# Result on aarch64 + Ubuntu Noble: uses the ubuntu_noble-specific dependencies
```

## Supported Architectures

- `x86_64` - 64-bit x86 (Intel/AMD)
- `i686` - 32-bit x86
- `aarch64` - 64-bit ARM
- `armv7h` - ARMv7 hard-float
- `armv6h` - ARMv6 hard-float
- `armv5` - ARMv5
- `ppc64` - PowerPC 64-bit
- `ppc64le` - PowerPC 64-bit Little Endian
- `s390x` - IBM System z
- `mips`, `mipsle` - MIPS architectures
- `riscv64` - RISC-V 64-bit
- `pentium4` - Pentium 4 optimized
- `any` - Architecture-independent

## Usage

```bash
# Build for current architecture
yap build

# YAP will automatically:
# 1. Detect the current architecture
# 2. Apply architecture-specific variables if they exist
# 3. Fall back to distribution-specific or base variables
# 4. Use the highest priority match
```

## Implementation Details

- Architecture detection uses `runtime.GOARCH` mapped to pacman architecture names
- Architecture-specific variables use single underscore (`_`) to avoid conflict with distribution variables (`__`)
- Variables are only applied when building for the matching architecture
- Invalid architectures are ignored and fall through to lower priority variables