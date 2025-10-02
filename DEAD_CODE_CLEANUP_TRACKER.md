# YAP Dead Code Cleanup & Improvements - Comprehensive Tracker

**Created:** 2025-10-02  
**Last Updated:** 2025-10-02  
**Session:** Phase 2 - Context Infrastructure Cleanup  

---

## 🎯 Executive Summary

### What We've Already Completed

**Phase 1 (Completed):**
- ✅ Removed 606 lines of dead code
- ✅ Fixed build tests to use correct error constructors
- ✅ All tests passing

**Phase 2 (Completed):**
- ✅ Removed 190 lines from pkg/context
- ✅ Removed TimeoutManager, Pool, and DoWith* functions
- ✅ All tests passing, linting clean

**Previous Work (Already Done):**
- ✅ Fixed all critical builder issues (APK stubs, RPM Update, Pacman constants)
- ✅ Consolidated common builder methods (~288 lines via BaseBuilder)
- ✅ Fixed APK tests (6 tests)
- ✅ Removed DEB processDepends wrapper
- ✅ Zero linting violations

### Current Status

**Total Dead Code Identified:** ~1,500+ lines  
**Lines Removed So Far:** 796 lines (53% complete)  
**Remaining Work:** ~710 lines to analyze and remove  

---

## 📊 Quick Stats

| Category | Lines Found | Lines Removed | Remaining | Status |
|----------|-------------|---------------|-----------|--------|
| **Phase 1** | 606 | 606 | 0 | ✅ Complete |
| **Phase 2 (context)** | 190 | 190 | 0 | ✅ Complete |
| **pkg/logger** | ~200 | 0 | ~200 | 📋 Planned |
| **pkg/errors** | 139 | 139 | 0 | ✅ Complete |
| **pkg/core** | ~180 | 0 | ~180 | 📋 Planned |
| **Other** | ~191 | ~42 | ~149 | 🔄 In Progress |
| **TOTAL** | **~1,506** | **~977** | **~529** | **65% Complete** |

---

## 🔍 Issues Identified & Categorized

### 1. COMPLETED ✅

#### 1.1 Phase 1: Error, Archive, and Test Code (606 lines)
**Status:** ✅ COMPLETED  
**Completion Date:** 2025-10-02  
**Commit:** `3dae7ba`

**Removed:**
- `pkg/errors/errors.go`: 139 lines
  - Formatted error constructors (Newf, Wrapf)
  - 8 typed error helpers (NewValidationError, NewFileSystemError, etc.)
  - IsType(), GetContext()
  - ChainError struct and all methods
- `pkg/archive/tar.go`: 43 lines (CreateTarGz function)
- `pkg/source/source_test.go`: 262 lines (concurrent download tests)
- `pkg/archive/tar_test.go`: 42 lines
- `pkg/errors/errors_test.go`: 116 lines
- `cmd/yap/command/build_test.go`: 4 lines changed (updated to use New())

**Verification:**
- ✅ All tests passing
- ✅ No compilation errors
- ✅ ripgrep verified zero usage of removed functions

#### 1.2 Phase 2: Context Infrastructure Cleanup (190 lines)
**Status:** ✅ COMPLETED  
**Completion Date:** 2025-10-02  
**Commit:** `9265e70`

**Removed:**
- `pkg/context/context.go`: 145 lines
  - TimeoutManager struct + 6 methods (NewTimeoutManager, AddTimeout, CancelTimeout, CancelAll, GetActiveTimeouts, ListActive)
  - timeoutEntry struct
  - Pool struct + 3 methods (NewPool, Get, Put)
  - DoWithTimeout, DoWithDeadline, DoWithCancel utility functions
- `pkg/context/context_test.go`: 45 lines
  - TestTimeoutManager
  - TestDoWithTimeout

**Kept (Required by pkg/download):**
- Semaphore (used by WorkerPool)
- WorkerPool (used by ConcurrentDownloadManager)
- Note: These will be removed in Phase 5 after ConcurrentDownloadManager cleanup

**Verification:**
- ✅ All tests passing (make test)
- ✅ Linting clean (make lint)
- ✅ Zero references found to removed code

---

### 2. HIGH PRIORITY - Dead Infrastructure 🔴

#### 2.1 pkg/context/context.go - Remaining Unused Code (~210 lines)

**Status:** 📋 PLANNED (Phase 2 PARTIAL - removed 190 lines)  
**Priority:** MEDIUM  
**Estimated Effort:** 1-2 hours  
**Risk:** LOW (unused code)

**Phase 2 Completed (190 lines removed):**
- ✅ TimeoutManager + timeoutEntry structs (removed)
- ✅ Pool struct (removed)
- ✅ DoWithTimeout, DoWithDeadline, DoWithCancel (removed)
- ✅ GetActiveTimeouts/ListActive duplicates (removed)

**Still Remaining Dead Code (~210 lines):**

1. **Build Context Management (NOT USED):**
   - `NewBuildContext()` - line 50
   - `WithBuildContext()` - line 63
   - `GetBuildContext()` - line 75
   - BuildContext struct
   - Related context key constants

2. **Context Wrapper Functions (NOT USED):**
   - `WithTimeout()` - Simple wrapper around context.WithTimeout
   - `WithDeadline()` - Simple wrapper around context.WithDeadline
   - `WithCancel()` - Simple wrapper around context.WithCancel
   - `BackgroundWithTimeout()` - Helper function

3. **Context Key Functions (NOT USED):**
   - `WithLogger()` / `GetLogger()`
   - `WithTraceID()` / `GetTraceID()`
   - `WithRequestID()` / `GetRequestID()`
   - `WithOperation()` / `GetOperation()`

**Note on WorkerPool/Semaphore:**
These are currently KEPT because they're used by `pkg/download/download.go` ConcurrentDownloadManager. However, ConcurrentDownloadManager itself is never used (verified). These will be removed in Phase 5 after removing ConcurrentDownloadManager.

**Verification Commands:**
```bash
rg "NewBuildContext|WithBuildContext|GetBuildContext" --type go -g '!pkg/context/*' -g '!*_test.go'
rg "WithLogger|GetLogger" --type go -g '!pkg/context/*' -g '!*_test.go'
rg "WithTraceID|GetTraceID|WithRequestID|GetRequestID" --type go -g '!pkg/context/*' -g '!*_test.go'
```

**Recommendation:**
- **Phase 3:** Remove unused Build Context and Context Key functions (~210 lines)
- **Phase 5:** Remove WorkerPool/Semaphore after ConcurrentDownloadManager cleanup

---

#### 2.2 pkg/logger/logger.go - Unused Logger Features (~200 lines)

**Status:** 📋 PLANNED  
**Priority:** HIGH  
**Estimated Effort:** 2 hours  
**Risk:** LOW

**Dead Code Identified:**

1. **ComponentLogger (ENTIRE STRUCT NOT USED - ~80 lines):**
   - Type definition at line 229
   - 9 methods: Info(), Warn(), Error(), Fatal(), Debug(), etc.
   - Never instantiated or used in codebase

2. **CompatLogger (ENTIRE STRUCT NOT USED - ~90 lines):**
   - Type definition at line 348
   - 11 methods for legacy compatibility
   - Never used (legacy support not needed)

3. **Unused Logger Functions (~30 lines):**
   - `DefaultConfig()` - Config helper
   - `New()` / `NewDefault()` - Constructor alternatives
   - `ServiceLogger()` - Service-level logger
   - `SetGlobal()` / `Global()` - Global logger management
   - `WithError()` / `WithFields()` - Field helpers

4. **Style Customization Functions:**
   - `YapLogger.WithKeyStyles()`
   - `YapLogger.AppendKeyStyle()`
   - `ComponentLogger.WithKeyStyles()`
   - `ComponentLogger.AppendKeyStyle()`

**Current Usage:**
- Code uses global `logger.Logger` directly
- Uses: `Info()`, `Error()`, `Warn()`, `Fatal()`, `Debug()` from global
- Does NOT use ComponentLogger or CompatLogger

**Verification Commands:**
```bash
rg "ComponentLogger|NewComponentLogger" --type go -g '!pkg/logger/*' -g '!*_test.go'
rg "CompatLogger|NewCompatLogger" --type go -g '!pkg/logger/*' -g '!*_test.go'
rg "ServiceLogger|SetGlobal|Global\(\)" --type go -g '!pkg/logger/*' -g '!*_test.go'
```

**Recommendation:**
- **Remove:** ComponentLogger (entire struct)
- **Remove:** CompatLogger (entire struct)
- **Remove:** Unused constructor and global management functions
- **Keep:** Core YapLogger and global logger instance

**Files to Modify:**
- `pkg/logger/logger.go` (remove ~200 lines)
- `pkg/logger/logger_test.go` (remove corresponding tests)

---

#### 2.3 pkg/core/packager.go - Complete Duplicate of BaseBuilder (~180 lines)

**Status:** 📋 PLANNED  
**Priority:** HIGH  
**Estimated Effort:** 1 hour  
**Risk:** LOW

**Dead Code Identified:**

**Entire File Not Used:**
- `BasePackageManager` struct
- 10+ methods that duplicate `pkg/builders/common.BaseBuilder`
- Never imported or used anywhere in codebase

**Analysis:**
- This appears to be an old implementation before BaseBuilder was created
- All functionality exists in `pkg/builders/common/interface.go`
- Zero usage across entire codebase

**Verification Commands:**
```bash
rg "core.BasePackageManager|core.NewBasePackageManager" --type go
rg "\"github.com/M0Rf30/yap/v2/pkg/core\"" --type go -g '!pkg/core/*'
```

**Recommendation:**
- **DELETE:** Entire `pkg/core/packager.go` file (~180 lines)
- **DELETE:** Corresponding test file
- **Reason:** Complete duplicate, zero usage

**Files to DELETE:**
- `pkg/core/packager.go`
- `pkg/core/packager_test.go` (if exists)

---

### 3. MEDIUM PRIORITY - Dead Feature Code 🟡

#### 3.1 pkg/download/download.go - ConcurrentDownloadManager (~150 lines)

**Status:** 📋 PLANNED  
**Priority:** MEDIUM  
**Estimated Effort:** 1 hour  
**Risk:** LOW

**Dead Code Identified:**

1. **ConcurrentDownloadManager (lines 333-400+):**
   - Struct definition at line 333
   - `NewConcurrentDownloadManager()`
   - `AddDownload()`, `Start()`, `Wait()`, `Stop()`
   - Never used in codebase

2. **Job struct (line 342):**
   - Only used by ConcurrentDownloadManager

**Current Usage:**
- Code uses simple `Download()` function directly
- No concurrent downloads in production

**Verification Commands:**
```bash
rg "ConcurrentDownloadManager|NewConcurrentDownloadManager" --type go -g '!pkg/download/*' -g '!*_test.go'
```

**Recommendation:**
- **Remove:** ConcurrentDownloadManager and Job structs
- **Keep:** Simple Download() function (actively used)

**Files to Modify:**
- `pkg/download/download.go` (remove ~150 lines)
- `pkg/download/download_test.go` (already removed related tests in Phase 1)

---

#### 3.2 pkg/set/set.go - Duplicate Helper Functions (~15 lines)

**Status:** 📋 PLANNED  
**Priority:** LOW  
**Estimated Effort:** 15 minutes  
**Risk:** VERY LOW

**Dead Code Identified:**

1. **Contains() function (standalone):**
   - Wrapper around `slices.Contains`
   - Never used (code uses `slices.Contains` directly)

**Verification Commands:**
```bash
rg "set.Contains\(" --type go -g '!pkg/set/*'
```

**Recommendation:**
- **Remove:** Standalone `Contains()` function
- **Keep:** Set struct and methods (actively used)

**Files to Modify:**
- `pkg/set/set.go` (remove ~5 lines)

---

### 4. DUPLICATED CODE 🔄

#### 4.1 pkg/context/context.go - Duplicate Methods

**Found:** `GetActiveTimeouts()` and `ListActive()` are IDENTICAL

**Location:**
- Lines 238-248: `GetActiveTimeouts()`
- Lines 251-261: `ListActive()`

**Impact:** 13 lines of duplicate code

**Recommendation:**
- Keep `GetActiveTimeouts()` (more descriptive name)
- Remove `ListActive()` (if either is kept after dead code removal)
- OR remove both if TimeoutManager is removed

---

### 5. TODOs & INCOMPLETE FEATURES 📝

#### 5.1 APK Builder Implementation

**Status:** ✅ DOCUMENTED (No action needed)  
**Location:** `pkg/builders/apk/builder.go`  
**Lines:** 80-122

**Current State:**
- Properly documented TODO comments
- Clear error messages on use
- Tests updated to expect errors
- Not blocking any functionality

**Action:** None (already properly handled in previous work)

---

## 🗺️ Implementation Roadmap

### Phase 2: Context Package Cleanup
**Target Date:** TBD  
**Estimated Time:** 2-3 hours  
**Lines to Remove:** ~400

**Tasks:**
1. [ ] Verify zero usage of context utilities
2. [ ] Remove TimeoutManager struct and methods
3. [ ] Remove Pool struct and methods
4. [ ] Remove Semaphore struct and methods
5. [ ] Remove WorkerPool struct and methods
6. [ ] Remove unused context wrapper functions
7. [ ] Remove build context management
8. [ ] Update/remove corresponding tests
9. [ ] Run full test suite
10. [ ] Commit changes

**Verification Script:**
```bash
# Run before removal to confirm zero usage
rg "TimeoutManager|NewPool|Semaphore|WorkerPool" --type go -g '!pkg/context/*' -g '!*_test.go'
```

---

### Phase 3: Logger Package Cleanup
**Target Date:** TBD  
**Estimated Time:** 2 hours  
**Lines to Remove:** ~200

**Tasks:**
1. [ ] Verify ComponentLogger not used
2. [ ] Verify CompatLogger not used
3. [ ] Remove ComponentLogger struct and methods
4. [ ] Remove CompatLogger struct and methods
5. [ ] Remove unused logger constructor functions
6. [ ] Update/remove corresponding tests
7. [ ] Run full test suite
8. [ ] Commit changes

**Verification Script:**
```bash
rg "ComponentLogger|CompatLogger|ServiceLogger" --type go -g '!pkg/logger/*' -g '!*_test.go'
```

---

### Phase 4: Core Package Removal
**Target Date:** TBD  
**Estimated Time:** 1 hour  
**Lines to Remove:** ~180

**Tasks:**
1. [ ] Verify pkg/core not imported anywhere
2. [ ] Delete pkg/core/packager.go
3. [ ] Delete pkg/core/packager_test.go (if exists)
4. [ ] Update go.mod if needed
5. [ ] Run full test suite
6. [ ] Commit changes

**Verification Script:**
```bash
rg "\"github.com/M0Rf30/yap/v2/pkg/core\"" --type go
```

---

### Phase 5: Download & Misc Cleanup
**Target Date:** TBD  
**Estimated Time:** 1-2 hours  
**Lines to Remove:** ~165

**Tasks:**
1. [ ] Remove ConcurrentDownloadManager from pkg/download
2. [ ] Remove Job struct from pkg/download
3. [ ] Remove Contains() from pkg/set
4. [ ] Update/remove corresponding tests
5. [ ] Run full test suite
6. [ ] Commit changes

---

## 📈 Progress Tracking

### Overall Progress
- **Total Lines Identified:** ~1,506
- **Total Lines Removed:** 606 (40%)
- **Remaining:** ~900 (60%)

### Phase Breakdown
| Phase | Status | Lines | Progress |
|-------|--------|-------|----------|
| Phase 1: Errors & Archive | ✅ Complete | 606 | 100% |
| Phase 2: Context Cleanup | 📋 Planned | ~400 | 0% |
| Phase 3: Logger Cleanup | 📋 Planned | ~200 | 0% |
| Phase 4: Core Removal | 📋 Planned | ~180 | 0% |
| Phase 5: Misc Cleanup | 📋 Planned | ~165 | 0% |

---

## 🧪 Testing Strategy

### Before Each Phase
1. Run verification commands to confirm zero usage
2. Check test files for dependencies
3. Review import statements

### After Each Phase
1. `make test` - Full test suite
2. `make lint` - Linting checks
3. `make build` - Verify build
4. Check for unintended side effects

### Continuous Verification
- Use ripgrep to verify zero usage
- Check import statements
- Review test coverage changes

---

## ⚠️ Risk Assessment

### Low Risk (Safe to Remove)
- ✅ Phase 1: Errors & Archive (COMPLETED)
- 🟢 Phase 2: Context utilities (not used)
- 🟢 Phase 3: Logger features (not used)
- 🟢 Phase 4: Core package (complete duplicate)
- 🟢 Phase 5: Download manager (not used)

### Medium Risk
- None identified

### High Risk
- None identified

### Mitigation Strategy
- Comprehensive testing after each phase
- Git commits for easy rollback
- Verification scripts before removal
- Keep test coverage high

---

## 📝 Code Quality Improvements Beyond Dead Code

### Potential Improvements Identified

1. **Architecture:**
   - ✅ BaseBuilder consolidation (already done)
   - ✅ Builder consistency (already done)

2. **Documentation:**
   - ✅ APK builder TODOs (already done)
   - Consider adding package-level documentation

3. **Error Handling:**
   - ✅ Using Wrap() pattern consistently
   - Custom error types well-defined

4. **Testing:**
   - ✅ 100% test pass rate
   - ✅ Proper skip logic for sudo tests
   - Tests for dead code can be removed

5. **Linting:**
   - ✅ Zero violations (already achieved)

---

## 🎯 Success Metrics

### Targets
- [ ] Remove 1,500+ lines of dead code
- [x] Maintain 100% test pass rate (currently achieved)
- [x] Zero linting violations (currently achieved)
- [ ] Zero regressions introduced
- [ ] Improve code maintainability

### Current Achievements
- ✅ 606 lines removed (40% of target)
- ✅ All tests passing
- ✅ Zero linting violations
- ✅ All critical issues resolved (previous work)
- ✅ BaseBuilder consolidation complete

---

## 📚 References

### Related Documents
- `CODE_QUALITY_IMPROVEMENTS.md` - Previous cleanup work (completed)
- `REFACTORING_PLAN.md` - Original refactoring analysis
- `REFACTORING_SUMMARY.md` - Summary of past work

### Commit History
- `3dae7ba` - Phase 1: Dead code removal (errors, archive, tests) - 606 lines

---

## 🔧 Useful Commands

### Find Usage
```bash
# Search for function usage (excluding tests and defining file)
rg "FunctionName" --type go -g '!*_test.go' -g '!defining_file.go'

# Find struct usage
rg "StructName\{|StructName\." --type go -g '!*_test.go'

# Find imports
rg "\"package/path\"" --type go
```

### Verification
```bash
# Count lines in file
wc -l file.go

# Check test coverage
go test -cover ./pkg/...

# Run specific package tests
go test ./pkg/context -v
```

### Before Commit
```bash
make test      # Full test suite
make lint      # Linting
make build     # Build verification
git diff --stat # Check changes
```

---

**Last Updated:** 2025-10-02  
**Next Review:** After Phase 2 completion  
**Maintained By:** Development Team
