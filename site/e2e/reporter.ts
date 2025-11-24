import * as fs from "node:fs/promises";
import type { Reporter, TestCase, TestResult } from "@playwright/test/reporter";
import { API } from "api/api";
import { coderdPProfPort } from "./constants";

class CoderReporter implements Reporter {
	async onTestEnd(test: TestCase, result: TestResult) {
		if (test.expectedStatus === "skipped") {
			return;
		}

		if (result.status === "passed") {
			return;
		}

		const fsTestTitle = test.title.replaceAll(" ", "-");
		const outputFile = `test-results/debug-pprof-goroutine-${fsTestTitle}.txt`;
		await exportDebugPprof(outputFile);

		console.info(`Data from pprof has been saved to ${outputFile}`);
		console.info("==> Output");
	}
}

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

export default CoderReporter;
