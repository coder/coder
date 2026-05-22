import type * as TypesGen from "#/api/typesGenerated";

type ParsedGoalCommand =
	| { kind: "show" }
	| {
			kind: "set";
			objective: string;
			mutation: TypesGen.ChatGoalMutation;
	  }
	| {
			kind: "lifecycle";
			action: Exclude<TypesGen.ChatGoalMutationAction, "set">;
			mutation: TypesGen.ChatGoalMutation;
	  }
	| { kind: "unsupported"; reason: string };

const commandPrefix = "/goal";
const turnCapFlagPattern = /^--(?:max-)?(?:turns?|turn-cap|turn-limit)\b/i;
const budgetFlagPattern = /^(?:--)?budget\b/i;

const completionSummaryPrefix = "complete --summary";

const makeLifecycleMutation = (
	action: Exclude<TypesGen.ChatGoalMutationAction, "set">,
	completionSummary?: string,
): ParsedGoalCommand => ({
	kind: "lifecycle",
	action,
	mutation:
		completionSummary === undefined
			? { action }
			: { action, completion_summary: completionSummary },
});

export const parseGoalCommand = (message: string): ParsedGoalCommand | null => {
	if (!message.startsWith(commandPrefix)) {
		return null;
	}

	const afterPrefix = message.slice(commandPrefix.length);
	if (afterPrefix.length > 0 && !/^\s/.test(afterPrefix)) {
		return null;
	}

	const args = afterPrefix.trim();
	if (!args) {
		return { kind: "show" };
	}

	const firstToken = args.split(/\s+/, 1)[0] ?? "";
	if (
		budgetFlagPattern.test(firstToken) ||
		turnCapFlagPattern.test(firstToken)
	) {
		return {
			kind: "unsupported",
			reason:
				"Goal budget and turn limit commands are not supported. Set only the objective.",
		};
	}

	if (args === "--" || args.startsWith("-- ") || args.startsWith("--\n")) {
		const escapedObjective = args.slice(2).trim();
		if (!escapedObjective) {
			return {
				kind: "unsupported",
				reason: "Provide an objective after /goal --.",
			};
		}
		return {
			kind: "set",
			objective: escapedObjective,
			mutation: { action: "set", objective: escapedObjective },
		};
	}

	const normalizedArgs = args.toLocaleLowerCase("en-US");
	if (normalizedArgs === "clear") {
		return makeLifecycleMutation("clear");
	}
	if (normalizedArgs === "pause") {
		return makeLifecycleMutation("pause");
	}
	if (normalizedArgs === "resume") {
		return makeLifecycleMutation("resume");
	}
	if (normalizedArgs === "complete") {
		return makeLifecycleMutation("complete");
	}
	if (
		normalizedArgs === completionSummaryPrefix ||
		normalizedArgs.startsWith(`${completionSummaryPrefix} `) ||
		normalizedArgs.startsWith(`${completionSummaryPrefix}\n`)
	) {
		const summary = args.slice(completionSummaryPrefix.length).trim();
		if (!summary) {
			return {
				kind: "unsupported",
				reason: "Provide a summary after /goal complete --summary.",
			};
		}
		return makeLifecycleMutation("complete", summary);
	}

	return {
		kind: "set",
		objective: args,
		mutation: { action: "set", objective: args },
	};
};
