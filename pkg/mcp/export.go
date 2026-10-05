package mcp

import "reflect"

// allFieldsBuildArgs returns a buildArgs with every field set to a non-zero
// value, so the generated argv exercises every flag the dispatcher can emit.
func allFieldsBuildArgs() *buildArgs {
	args := &buildArgs{}

	for _, f := range reflect.ValueOf(args).Elem().Fields() {
		switch f.Kind() { //nolint:exhaustive // only the kinds buildArgs uses
		case reflect.Bool:
			f.SetBool(true)
		case reflect.String:
			f.SetString("x")
		case reflect.Slice:
			f.Set(reflect.ValueOf([]string{"x"}))
		default:
			panic("mcp: unhandled buildArgs field kind " + f.Kind().String())
		}
	}

	return args
}

// ContainerBuildArgvSample returns the container-dispatched `yap build` argv
// generated from a fully populated build request. Intended for tests that
// check the argv against the real CLI flagset.
func ContainerBuildArgvSample() []string {
	return buildCLIArgsFromArgs(allFieldsBuildArgs(), "ubuntu-noble")
}

// ContainerPrepareArgvSample returns the chained `yap prepare` argv generated
// from a fully populated build request.
func ContainerPrepareArgvSample() []string {
	return prepareCLIArgs(allFieldsBuildArgs(), "ubuntu-noble")
}
