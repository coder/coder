/* eslint-disable no-console -- Logging is sort of the whole point here */
import type {
  FullConfig,
  Suite,
  TestCase,
  TestResult,
  FullResult,
  Reporter,
  TestError,
} from "@playwright/test/reporter";
import axios from "axios";
import * as fs from "fs/promises";
import type { Writable } from "stream";
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
    console.log(`==> Running ${suite.allTests().length} tests`);
  }

  onTestBegin(test: TestCase) {
    this.testOutput.set(test.id, []);
    console.log(`==> Starting test ${test.title}`);
  }

  onStdOut(chunk: string, test?: TestCase, _?: TestResult): void {
    // If there's no associated test, just print it now
    if (!test) {
      for (const line of logLines(chunk)) {
        console.log(`[stdout] ${line}`);
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
        console.log(`==> Skipping test ${test.title}`);
        this.skippedCount++;
        return;
      }

      console.log(`==> Finished test ${test.title}: ${result.status}`);

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

      console.log(`Data from pprof has been saved to ${outputFile}`);
      console.log("==> Output");
      const output = this.testOutput.get(test.id)!;
      for (const [target, chunk] of output) {
        target.write(`${chunk.replace(/\n$/g, "")}\n`);
      }

      if (result.errors.length > 0) {
        console.log("==> Errors");
        for (const error of result.errors) {
          reportError(error);
        }
      }

      if (result.attachments.length > 0) {
        console.log("==> Attachments");
        for (const attachment of result.attachments) {
          console.log(attachment);
        }
      }
    } finally {
      this.testOutput.delete(test.id);
    }
  }

  onEnd(result: FullResult) {
    console.log(`==> Tests ${result.status}`);
    if (!enterpriseLicense) {
      console.log(
        "==> Enterprise tests were skipped, because no license was provided",
      );
    }
    console.log(`${this.passedCount} passed`);
    if (this.skippedCount > 0) {
      console.log(`${this.skippedCount} skipped`);
    }
    if (this.failedTests.length > 0) {
      console.log(`${this.failedTests.length} failed`);
      for (const test of this.failedTests) {
        console.log(`  ${test.location.file} › ${test.title}`);
      }
    }
    if (this.timedOutTests.length > 0) {
      console.log(`${this.timedOutTests.length} timed out`);
      for (const test of this.timedOutTests) {
        console.log(`  ${test.location.file} › ${test.title}`);
      }
    }
  }
}

const logLines = (chunk: string): string[] => chunk.trimEnd().split("\n");

const exportDebugPprof = async (outputFile: string) => {
  const response = await axios.get(
    `http://127.0.0.1:${coderdPProfPort}/debug/pprof/goroutine?debug=1`,
  );
  if (response.status !== 200) {
    throw new Error(`Error: Received status code ${response.status}`);
  }

  await fs.writeFile(outputFile, response.data);
};

const reportError = (error: TestError) => {
  if (error.location) {
    console.log(`${error.location.file}:${error.location.line}:`);
  }
  if (error.snippet) {
    console.log(error.snippet);
  }

  if (error.message) {
    console.log(error.message);
  } else {
    console.log(error);
  }
};

// eslint-disable-next-line no-unused-vars -- Playwright config uses it
export default CoderReporter;
