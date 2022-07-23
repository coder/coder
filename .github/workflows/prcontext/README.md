# prcontext

`prcontext` is a simple Go program that extracts CI directives from PRs for a
more efficient merge cycle.

Since skips are only possible within PRs, the full suite will still run on
merge.
