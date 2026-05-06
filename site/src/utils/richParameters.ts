import type { WorkspaceBuildParameter } from "#/api/typesGenerated";

type AutofillSource = "user_history" | "url" | "active_build";

// AutofillBuildParameter is a build parameter destined to a form, alongside
// its source so that the form can explain where the value comes from.
export type AutofillBuildParameter = {
	source: AutofillSource;
} & WorkspaceBuildParameter;
