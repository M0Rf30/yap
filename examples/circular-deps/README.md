# Circular Dependency Example

This example demonstrates YAP's circular dependency detection and error handling.

## Structure

```
circular-deps/
├── yap.json                 # Package configuration
├── package-a/
│   ├── PKGBUILD            # Depends on package-b
│   └── package-a.c         # Calls package_b_function()
└── package-b/
    ├── PKGBUILD            # Depends on package-a  
    └── package-b.c         # Calls package_a_function()
```

## Circular Dependency

- **package-a** depends on **package-b** (in PKGBUILD `depends=('package-b')`)
- **package-b** depends on **package-a** (in PKGBUILD `depends=('package-a')`)

This creates an impossible dependency resolution scenario that YAP should detect and reject.

## Testing

Run this example to verify YAP's circular dependency detection:

```bash
yap build examples/circular-deps
```

## Expected Behavior

YAP should detect the circular dependency and return an error like:

```
Error: circular dependency detected: package-a -> package-b -> package-a
```

This error should be thrown by the dependency resolution logic in `/pkg/project/project.go` at the `ErrCircularDependency` check.

## Purpose

This example validates that YAP properly:

1. **Detects circular dependencies** during build planning
2. **Prevents infinite loops** in dependency resolution
3. **Provides clear error messages** to help developers identify the problem
4. **Fails fast** before attempting any builds that would be impossible to complete
