# Expected Output Analysis

When you run the dependency orchestration example, you'll see this exact output pattern:

## Dependency Analysis (Automatic Detection)

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

## Dependency Popularity Analysis

```
foundation-lib: depended on by 4 packages 
database-client: depended on by 1 packages 
web-framework: depended on by 1 packages
```

## Build Order (Topological Sort)

```
Batch 1: foundation-lib(deps:4)                    # Most fundamental
Batch 2: web-framework(deps:1) database-client(deps:1) middleware-service(deps:0)  # Parallel
Batch 3: main-application(deps:0)                  # Final application
```

## Installation Strategy

```
runtime dependency map: 
foundation-lib -> WILL BE INSTALLED (runtime dependency)
web-framework -> WILL BE INSTALLED (runtime dependency)
database-client -> WILL BE INSTALLED (runtime dependency)
```

## Key Insights

1. **Zero Manual Configuration**: No `"install": true` flags were needed
2. **Intelligent Dependency Detection**: YAP automatically identified which packages need installation
3. **Optimal Build Order**: Most depended-upon packages (foundation-lib) built first
4. **Parallel Processing**: Packages with no interdependencies (batch 2) build in parallel
5. **Installation Between Batches**: Runtime dependencies installed immediately after building

This demonstrates YAP's sophisticated dependency orchestration capabilities.