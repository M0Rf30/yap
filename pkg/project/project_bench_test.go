package project_test

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/builder"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
	"github.com/M0Rf30/yap/v2/pkg/project"
)

// TestMain silences logger output so benchmark measurements aren't polluted
// by per-iteration Info log lines from BuildAll.
func TestMain(m *testing.M) {
	logger.SetWriter(io.Discard)
	os.Exit(m.Run())
}

// BenchmarkTopologicalSort50Packages benchmarks topological sort with 50 packages.
// Note: This benchmark tests the dependency resolution logic by calling BuildAll
// with Parallel=true, which internally uses topologicalSort.
func BenchmarkTopologicalSort50Packages(b *testing.B) {
	mpc := createMultipleProjectWithDeps(50)
	mpc.Opts.Parallel = true

	b.ResetTimer()

	for range b.N {
		// BuildAll with Parallel=true triggers topologicalSort internally
		// We ignore the error since we're benchmarking the sort, not the build
		_ = mpc.BuildAll()
	}
}

// BenchmarkTopologicalSort200Packages benchmarks topological sort with 200 packages.
// Note: This benchmark tests the dependency resolution logic by calling BuildAll
// with Parallel=true, which internally uses topologicalSort.
func BenchmarkTopologicalSort200Packages(b *testing.B) {
	mpc := createMultipleProjectWithDeps(200)
	mpc.Opts.Parallel = true

	b.ResetTimer()

	for range b.N {
		// BuildAll with Parallel=true triggers topologicalSort internally
		// We ignore the error since we're benchmarking the sort, not the build
		_ = mpc.BuildAll()
	}
}

// createMultipleProjectWithDeps creates a MultipleProject with N projects
// and realistic dependency density (~3 deps per package drawn from earlier indices).
func createMultipleProjectWithDeps(count int) *project.MultipleProject {
	mpc := &project.MultipleProject{
		Name:        "benchmark-project",
		Description: "Benchmark project",
		BuildDir:    "/tmp/build",
		Output:      "/tmp/output",
		Projects:    make([]*project.Project, count),
	}

	// Create projects with realistic dependencies
	for i := range count {
		pkgName := fmt.Sprintf("pkg-%d", i)
		pb := &pkgbuild.PKGBUILD{
			PkgName: pkgName,
			PkgVer:  "1.0",
			PkgRel:  "1",
		}

		// Add ~3 dependencies from earlier packages (creating a DAG)
		// This simulates realistic dependency density
		if i > 0 {
			// Depend on previous package
			pb.Depends = append(pb.Depends, fmt.Sprintf("pkg-%d", i-1))
		}

		if i > 2 {
			// Depend on package 2 steps back
			pb.Depends = append(pb.Depends, fmt.Sprintf("pkg-%d", i-2))
		}

		if i > 5 {
			// Depend on package 5 steps back
			pb.Depends = append(pb.Depends, fmt.Sprintf("pkg-%d", i-5))
		}

		prj := &project.Project{
			Name: pkgName,
			Path: fmt.Sprintf("/tmp/pkg-%d", i),
			Builder: &builder.Builder{
				PKGBUILD: pb,
			},
		}

		mpc.Projects[i] = prj
	}

	return mpc
}
