# YAP Dependency Orchestration Example

This example demonstrates YAP's sophisticated **automatic dependency resolution** and **installation orchestration** capabilities in complex multi-package projects.

## Overview

This project contains 5 interconnected packages that showcase:

- **Automatic Installation Detection**: YAP automatically determines which packages need to be installed based on internal dependencies
- **Dependency-Aware Build Ordering**: Packages are built in topologically sorted order
- **Installation Orchestration Between Batches**: Runtime dependencies are installed immediately after building, before dependent packages are built
- **No Manual Installation Flags Required**: The `"install": true` flag is optional - YAP derives installation requirements from dependency analysis

## Package Dependencies

```
foundation-lib (no dependencies)
    ↑
    ├── middleware-service (depends on foundation-lib)
    ├── database-client (depends on foundation-lib)
    └── web-framework (depends on foundation-lib)
            ↑
            └── main-application (depends on web-framework, database-client, foundation-lib)
```

## Dependency Resolution Result

YAP will automatically determine:

1. **Build Order**: 
   - **Batch 1**: `foundation-lib` (most fundamental, depended on by 4 packages)
   - **Batch 2**: `middleware-service`, `database-client`, `web-framework` (parallel, all depend only on foundation-lib)
   - **Batch 3**: `main-application` (depends on packages from batch 2)

2. **Installation Strategy**:
   - **foundation-lib**: ✅ **Will be installed** (runtime dependency of 4 packages)
   - **middleware-service**: ❌ Build only (no other packages depend on it)
   - **database-client**: ✅ **Will be installed** (runtime dependency of main-application)
   - **web-framework**: ✅ **Will be installed** (runtime dependency of main-application)
   - **main-application**: ❌ Build only (no other packages depend on it)

## Expected Output

When you run `yap build examples/dependency-orchestration/`, you'll see:

```
📦 foundation-lib-1.0.0-1
└─ Will be installed after build (runtime dependency)

📦 middleware-service-1.0.0-1  
  ├─ Runtime Dependencies:
  │  └─ foundation-lib (internal)
└─ Build only (no installation)

📦 database-client-2.0.0-1
  ├─ Runtime Dependencies:
  │  └─ foundation-lib (internal)
└─ Will be installed after build (runtime dependency)

📦 web-framework-3.1.0-1
  ├─ Runtime Dependencies:
  │  └─ foundation-lib (internal)
└─ Will be installed after build (runtime dependency)

📦 main-application-1.5.0-1
  ├─ Runtime Dependencies:
  │  ├─ web-framework (internal)
  │  ├─ database-client (internal)
  │  └─ foundation-lib (internal)
└─ Build only (no installation)
```

And dependency popularity analysis:
```
foundation-lib: depended on by 4 packages
web-framework: depended on by 1 packages
database-client: depended on by 1 packages
```

## Running the Example

```bash
# Build with dependency orchestration (requires root for actual installation)
yap build examples/dependency-orchestration/

# Build without installation (for testing dependency analysis)
yap build examples/dependency-orchestration/ --skip-sync --nomakedeps

# Verbose mode to see detailed dependency resolution
yap build examples/dependency-orchestration/ -v --skip-sync --nomakedeps
```

## Key Learning Points

1. **No `"install": true` flags needed** - YAP automatically determines installation requirements
2. **Topological sorting** ensures dependencies are built before dependents
3. **Installation happens between batches** - runtime dependencies are installed immediately after building
4. **Parallel processing** within batches for packages with no interdependencies
5. **Dependency popularity** determines build order within batches (most fundamental packages first)

This demonstrates YAP's ability to handle complex, real-world dependency scenarios without manual configuration.