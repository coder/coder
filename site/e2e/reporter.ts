import * as fs from "node:fs/promises";
import type { Writable } from "node:stream";
import type {
	FullConfig,
	FullResult,
	Reporter,
	Suite,
	TestCase,
	TestError,
	TestResult,
} from "@playwright/test/reporter";
import { API } from "api/api";
import { coderdPProfPort, enterpriseLicense } from "./constants";

class CoderReporter implements Reporter {
	config: FullConfig | null = null;
	testOutput = new Map<string, Array<[Writable, string]>>();
	passedCount = 0;
	skippedCount = 0;
	failedTests: TestCase[] = [];
	timedOutTests: TestCase[] = [];

	onBegin(config: FullConfig, suite: Suite) {
		this.config = config;
		console.info(`==> Running ${suite.allTests().length} tests`);
	}

	onTestBegin(test: TestCase) {
		this.testOutput.set(test.id, []);
		console.info(`==> Starting test ${test.title}`);
	}

	onStdOut(chunk: string, test?: TestCase, _?: TestResult): void {
		// If there's no associated test, just print it now
		if (!test) {
			for (const line of logLines(chunk)) {
				console.info(`[stdout] ${line}`);
			}
			return;
		}
		// Will be printed if the test fails
		this.testOutput.get(test.id)!.push([process.stdout, chunk]);
	}

	onStdErr(chunk: string, test?: TestCase, _?: TestResult): void {
		// If there's no associated test, just print it now
		if (!test) {
			for (const line of logLines(chunk)) {
				console.error(`[stderr] ${line}`);
			}
			return;
		}
		// Will be printed if the test fails
		this.testOutput.get(test.id)!.push([process.stderr, chunk]);
	}

	async onTestEnd(test: TestCase, result: TestResult) {
		try {
			if (test.expectedStatus === "skipped") {
				console.info(`==> Skipping test ${test.title}`);
				this.skippedCount++;
				return;
			}

			console.info(`==> Finished test ${test.title}: ${result.status}`);

			if (result.status === "passed") {
				this.passedCount++;
				return;
			}

			if (result.status === "failed") {
				this.failedTests.push(test);
			}

			if (result.status === "timedOut") {
				this.timedOutTests.push(test);
			}

			const fsTestTitle = test.title.replaceAll(" ", "-");
			const outputFile = `test-results/debug-pprof-goroutine-${fsTestTitle}.txt`;
			await exportDebugPprof(outputFile);

			console.info(`Data from pprof has been saved to ${outputFile}`);
			console.info("==> Output");
			const output = this.testOutput.get(test.id)!;
			for (const [target, chunk] of output) {
				target.write(`${chunk.replace(/\n$/g, "")}\n`);
			}

			if (result.errors.length > 0) {
				console.info("==> Errors");
				for (const error of result.errors) {
					reportError(error);
				}
			}

			if (result.attachments.length > 0) {
				console.info("==> Attachments");
				for (const attachment of result.attachments) {
					console.info(attachment);
				}
			}
		} finally {
			this.testOutput.delete(test.id);
		}
	}

	onEnd(result: FullResult) {
		console.info(`==> Tests ${result.status}`);
		if (!enterpriseLicense) {
			console.info(
				"==> Enterprise tests were skipped, because no license was provided",
			);
		}
		console.info(`${this.passedCount} passed`);
		if (this.skippedCount > 0) {
			console.info(`${this.skippedCount} skipped`);
		}
		if (this.failedTests.length > 0) {
			console.info(`${this.failedTests.length} failed`);
			for (const test of this.failedTests) {
				console.info(`  ${test.location.file} › ${test.title}`);
			}
		}
		if (this.timedOutTests.length > 0) {
			console.info(`${this.timedOutTests.length} timed out`);
			for (const test of this.timedOutTests) {
				console.info(`  ${test.location.file} › ${test.title}`);
			}
		}
	}
}

const logLines = (chunk: string | Buffer): string[] => {
	if (chunk instanceof Buffer) {
		// When running in a debugger, the input to this is a Buffer instead of a string.
		// Unsure why, but this prevents the `trimEnd` from throwing an error.
		return [chunk.toString()];
	}
	return chunk.trimEnd().split("\n");
};

const exportDebugPprof = async (outputFile: string) => {
	const axiosInstance = API.getAxiosInstance();
	const response = await axiosInstance.get(
		`http://127.0.0.1:${coderdPProfPort}/debug/pprof/goroutine?debug=1`,
	);

	if (response.status !== 200) {
		throw new Error(`Error: Received status code ${response.status}`);
	}

	await fs.writeFile(outputFile, response.data);
};

const reportError = (error: TestError) => {
	if (error.location) {
		console.info(`${error.location.file}:${error.location.line}:`);
	}
	if (error.snippet) {
		console.info(error.snippet);
	}

	if (error.message) {
		console.info(error.message);
	} else {
		console.info(error);
	}
};

export default CoderReporter;
