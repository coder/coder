# prcontext

`prcontext` is a simple Go program that extracts CI directives from PRs for a
more efficient merge cycle.

Right now it only supports the `[ci-skip [job ...]]` directive. Since skips are
only possible within PRs, the full suite will still run on merge.
