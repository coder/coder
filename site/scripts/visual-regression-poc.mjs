import { spawnSync } from "node:child_process";
import { appendFileSync, cpSync, existsSync, mkdirSync, rmSync } from "node:fs";
import path from "node:path";
import { compare as compareImages } from "reg-cli";

const storyFiles = [
	"src/components/PasswordField/PasswordField.stories.tsx",
	"src/modules/dashboard/LicenseBanner/LicenseBannerView.stories.tsx",
];

const paths = {
	actual: "test-results/storybook-snapshots",
	baseline: "visual-baselines/storybook-poc",
	expected: "test-results/visual-regression-poc/expected",
	diff: "test-results/visual-regression-poc/diff",
	json: "test-results/visual-regression-poc/reg.json",
	report: "test-results/visual-regression-poc/report.html",
};

const run = (command, args, options = {}) => {
	const result = spawnSync(command, args, {
		stdio: "inherit",
		shell: process.platform === "win32",
		...options,
	});
	if (result.status !== 0) {
		process.exit(result.status ?? 1);
	}
};

const clean = (dir) => {
	rmSync(dir, { force: true, recursive: true });
};

const capture = () => {
	clean(paths.actual);
	run("pnpm", [
		"vitest",
		"run",
		"--project=storybook",
		...storyFiles,
	], {
		env: {
			...process.env,
			STORYBOOK: "true",
			VISUAL_REGRESSION: "true",
		},
	});
};

const makeBaseline = () => {
	capture();
	clean(paths.baseline);
	mkdirSync(path.dirname(paths.baseline), { recursive: true });
	cpSync(paths.actual, paths.baseline, { recursive: true });
};

const summarize = (result) => {
	const changed = result.failedItems?.length ?? 0;
	const added = result.newItems?.length ?? 0;
	const deleted = result.deletedItems?.length ?? 0;
	const passed = result.passedItems?.length ?? 0;
	const total = changed + added + deleted + passed;
	const reportPath = path.relative(process.cwd(), paths.report);
	const imageRows = [
		...(result.failedItems ?? []).map((item) => ["Changed", item]),
		...(result.newItems ?? []).map((item) => ["Added", item]),
		...(result.deletedItems ?? []).map((item) => ["Deleted", item]),
	];
	const summary = [
		"## Visual regression PoC",
		"",
		`| Result | Count |`,
		"| --- | ---: |",
		`| Changed | ${changed} |`,
		`| Added | ${added} |`,
		`| Deleted | ${deleted} |`,
		`| Passed | ${passed} |`,
		`| Total | ${total} |`,
		"",
		`HTML report artifact path: \`${reportPath}\``,
		"Download the uploaded visual regression artifact to inspect the HTML report, baseline snapshots, actual snapshots, and diffs.",
		"",
		...(imageRows.length > 0
			? [
					"### Images with visual differences",
					"",
					"| Result | Image |",
					"| --- | --- |",
					...imageRows.map(([status, item]) => `| ${status} | \`${item}\` |`),
					"",
				]
			: []),
	].join("\n");

	console.log(summary);
	if (process.env.GITHUB_STEP_SUMMARY) {
		appendFileSync(process.env.GITHUB_STEP_SUMMARY, summary);
	}
	return { added, changed, deleted };
};

const statusLabel = (type) => {
	if (type === "pass") {
		return "✔ pass";
	}
	if (type === "change" || type === "fail") {
		return "✘ change";
	}
	if (type === "new") {
		return "+ added";
	}
	if (type === "deleted") {
		return "✘ deleted";
	}
	return type;
};

const compareSnapshots = () => {
	return new Promise((resolve, reject) => {
		const comparison = compareImages({
			actualDir: paths.actual,
			diffDir: paths.diff,
			expectedDir: paths.expected,
			extendedErrors: true,
			json: paths.json,
			report: paths.report,
		});

		comparison.on("compare", ({ path: imagePath, type }) => {
			console.log(`${statusLabel(type).padEnd(10)} ${path.join(paths.actual, imagePath)}`);
		});
		comparison.on("complete", resolve);
		comparison.on("error", reject);
	});
};

const compare = async () => {
	if (!existsSync(paths.baseline)) {
		throw new Error(`Missing baseline directory: ${paths.baseline}`);
	}
	if (!existsSync(paths.actual)) {
		throw new Error(`Missing actual directory: ${paths.actual}`);
	}
	clean(path.dirname(paths.diff));
	mkdirSync(path.dirname(paths.expected), { recursive: true });
	cpSync(paths.baseline, paths.expected, { recursive: true });

	const result = await compareSnapshots();
	const { added, changed, deleted } = summarize(result);
	if (added > 0 || changed > 0 || deleted > 0) {
		process.exit(1);
	}
};

const main = async () => {
	const command = process.argv[2];

	if (command === "capture") {
		capture();
	} else if (command === "baseline") {
		makeBaseline();
	} else if (command === "compare") {
		await compare();
	} else if (command === "run") {
		capture();
		await compare();
	} else {
		console.error("Usage: node scripts/visual-regression-poc.mjs <capture|baseline|compare|run>");
		process.exit(1);
	}
};

try {
	await main();
} catch (error) {
	console.error(error);
	process.exit(1);
}
