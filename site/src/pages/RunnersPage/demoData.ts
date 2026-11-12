import * as Yup from "yup";

/** Templates available in the runner demo, independent of deployment data. */
export const demoTemplates = [
	{
		name: "coder/aws-linux-ci",
		displayName: "AWS Linux CI",
		icon: "/icon/aws.svg",
		presets: ["small", "medium", "large"],
	},
	{
		name: "coder/docker-ci",
		displayName: "Docker CI",
		icon: "/icon/docker.svg",
		presets: ["standard", "performance"],
	},
];

/** Shared validation for the mock runner form and persisted configuration. */
export const runnerValuesSchema = Yup.object({
	label: Yup.string()
		.required("Enter a runner label.")
		.matches(
			/^[a-zA-Z0-9][a-zA-Z0-9._-]*$/,
			"Use letters, numbers, periods, underscores, and hyphens.",
		),
	template: Yup.string()
		.required("Choose a template.")
		.oneOf(demoTemplates.map((template) => template.name)),
	preset: Yup.string()
		.required("Choose a preset.")
		.test(
			"template-preset",
			"Choose a preset for this template.",
			function (value) {
				return (
					demoTemplates
						.find((template) => template.name === this.parent.template)
						?.presets.includes(value ?? "") ?? false
				);
			},
		),
	poolSize: Yup.number()
		.typeError("Enter a whole number.")
		.integer("Enter a whole number.")
		.min(0, "Pool size cannot be negative.")
		.max(Yup.ref("maxRunners"), "Pool size cannot exceed the maximum.")
		.required("Enter a pool size."),
	maxRunners: Yup.number()
		.typeError("Enter a whole number.")
		.integer("Enter a whole number.")
		.min(1, "Maximum runners must be at least 1.")
		.required("Enter a maximum."),
});

/** Editable configuration for a demo runner. */
export type RunnerValues = Yup.InferType<typeof runnerValuesSchema>;
/** A persisted demo runner, identified independently of its editable label. */
export type DemoRunner = RunnerValues & { id: string };

const storedRunnersSchema = Yup.array()
	.of(runnerValuesSchema.shape({ id: Yup.string().required() }))
	.required()
	.test(
		"unique",
		"Runner IDs and labels must be unique.",
		(runners) =>
			new Set(runners?.map((runner) => runner.id)).size === runners?.length &&
			new Set(runners?.map((runner) => runner.label.toLowerCase())).size ===
				runners?.length,
	);

/** Browser-only storage used exclusively by the runner demo. */
export const runnersStorageKey = "coder.demo.github-runners.v1";

/** Initial configurations from coder-gh-runners/config.yaml. */
const initialRunners: DemoRunner[] = [
	{
		id: "coder-small",
		label: "coder-small",
		template: "coder/aws-linux-ci",
		preset: "small",
		poolSize: 0,
		maxRunners: 10,
	},
	{
		id: "coder-large",
		label: "coder-large",
		template: "coder/aws-linux-ci",
		preset: "large",
		poolSize: 0,
		maxRunners: 4,
	},
];

/** Restores valid configuration, including an intentionally empty list. */
export function loadRunners(): {
	runners: DemoRunner[];
	storageUnavailable: boolean;
} {
	let stored: string | null;
	try {
		stored = localStorage.getItem(runnersStorageKey);
	} catch {
		return { runners: initialRunners, storageUnavailable: true };
	}
	if (stored !== null) {
		try {
			return {
				runners: storedRunnersSchema.validateSync(JSON.parse(stored), {
					strict: true,
				}),
				storageUnavailable: false,
			};
		} catch {
			// An older or malformed demo configuration is replaced on the next save.
		}
	}
	return { runners: initialRunners, storageUnavailable: false };
}

/** Current synthetic capacity, bounded by the runner's configured maximum. */
export function getRunnerSnapshot(runner: DemoRunner) {
	const hasActivity = initialRunners.some(
		(initial) => initial.id === runner.id,
	);
	const busy = hasActivity
		? Math.min(runner.id === "coder-small" ? 3 : 1, runner.maxRunners)
		: 0;
	const idle = Math.min(runner.poolSize, runner.maxRunners - busy);
	const starting =
		hasActivity && runner.id === "coder-small"
			? Math.min(1, runner.maxRunners - busy - idle)
			: 0;
	return { busy, idle, starting, hasActivity };
}

/** Synthetic history for the selected time range. */
export function getRunnerHistory(
	runner: DemoRunner,
	end: number,
	days: number,
) {
	const snapshot = getRunnerSnapshot(runner);
	const points = days === 1 ? 49 : days * 4 + 1;
	return Array.from({ length: points }, (_, index) => {
		const date =
			end - ((points - 1 - index) / (points - 1)) * days * 86_400_000;
		if (index === points - 1 || !snapshot.hasActivity)
			return {
				date,
				busy: snapshot.busy,
				idle: snapshot.idle,
				starting: snapshot.starting,
			};
		const wave = (Math.sin(index * 0.47) + Math.sin(index * 1.31) + 2) / 4;
		const busy = Math.min(
			runner.maxRunners,
			Math.round(wave * runner.maxRunners * 0.8),
		);
		const idle = Math.min(runner.poolSize, runner.maxRunners - busy);
		const starting =
			index % 7 === 0 ? Math.min(1, runner.maxRunners - busy - idle) : 0;
		return { date, busy, idle, starting };
	});
}

/** A fictional GitHub Actions job displayed by the demo. */
export type DemoJob = {
	id: number;
	name: string;
	workflow: string;
	branch: string;
	status: "In progress" | "Succeeded" | "Failed";
	queueSeconds: number;
	durationSeconds: number;
	startedAt: number;
};

/** Generates repeatable job history; new runners have no jobs. */
export function getRunnerJobs(
	runner: DemoRunner,
	end: number,
	days: number,
): DemoJob[] {
	const snapshot = getRunnerSnapshot(runner);
	if (!snapshot.hasActivity) return [];
	const names = [
		"Go tests",
		"Build frontend",
		"Lint",
		"Integration tests",
		"Build containers",
		"Typecheck",
	];
	return Array.from({ length: days * 48 }, (_, index) => ({
		id: 18432 - index,
		name: names[index % names.length] ?? "Build",
		workflow: index % 3 === 0 ? "Pull request checks" : "CI",
		branch: index % 3 === 0 ? "feat/workspace-insights" : "main",
		status:
			index < snapshot.busy
				? "In progress"
				: index % 19 === 0
					? "Failed"
					: "Succeeded",
		queueSeconds: 4 + ((index * 7) % 31),
		durationSeconds: 98 + ((index * 137) % 900),
		startedAt:
			end - (index < snapshot.busy ? (index + 1) * 60_000 : index * 1_800_000),
	}));
}
