/* eslint-disable no-console -- Logging is sort of the whole point here */
import * as fs from "fs";
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
import type { Writable } from "stream";

class CoderReporter implements Reporter {
  config: FullConfig | null = null;
  testOutput = new Map<string, Array<[Writable, string]>>();
  passedCount = 0;
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
    if (!test) {
      const preserve = this.config?.preserveOutput === "always";
      if (preserve) {
        console.log(`[stdout] ${chunk.replace(/\n$/g, "")}`);
      }
      return;
    }
    this.testOutput.get(test.id)!.push([process.stdout, chunk]);
  }

  onStdErr(chunk: string, test?: TestCase, _?: TestResult): void {
    if (!test) {
      const preserve = this.config?.preserveOutput === "always";
      if (preserve) {
        console.error(`[stderr] ${chunk.replace(/\n$/g, "")}`);
      }
      return;
    }
    this.testOutput.get(test.id)!.push([process.stderr, chunk]);
  }

  async onTestEnd(test: TestCase, result: TestResult) {
    console.log(`==> Finished test ${test.title}: ${result.status}`);

    if (result.status === "passed") {
      this.passedCount++;
    }

    if (result.status === "failed") {
      this.failedTests.push(test);
    }

    if (result.status === "timedOut") {
      this.timedOutTests.push(test);
    }

    const preserve = this.config?.preserveOutput;
    const logOutput =
      preserve === "always" ||
      (result.status !== "passed" && preserve !== "never");
    if (logOutput) {
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
    }
    this.testOutput.delete(test.id);

    await exportDebugPprof(test.title);
  }

  onEnd(result: FullResult) {
    console.log(`==> Tests ${result.status}`);
    console.log(`${this.passedCount} passed`);
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

const exportDebugPprof = async (testName: string) => {
  const url = "http://127.0.0.1:6060/debug/pprof/goroutine?debug=1";
  const outputFile = `test-results/debug-pprof-goroutine-${testName}.txt`;

  await axios
    .get(url)
    .then((response) => {
      if (response.status !== 200) {
        throw new Error(`Error: Received status code ${response.status}`);
      }

      fs.writeFile(outputFile, response.data, (err) => {
        if (err) {
          throw new Error(`Error writing to ${outputFile}: ${err.message}`);
        } else {
          console.log(`Data from ${url} has been saved to ${outputFile}`);
        }
      });
    })
    .catch((error) => {
      throw new Error(`Error: ${error.message}`);
    });
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
