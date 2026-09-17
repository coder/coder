// bippy's root entry imports `react` for its `useFiber` hook, which the
// overlay never calls. The build aliases `react` to this empty module so
// the bundle stays framework-free and works in pages without React.
export {};
