/* eslint-disable no-console -- Logging is sort of the whole point here */
import * as fs from "fs/promises";
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
      for (const line of filteredServerLogLines(chunk)) {
        console.log(`[stdout] ${line}`);
      }
      return;
    }
    this.testOutput.get(test.id)!.push([process.stdout, chunk]);
  }

  onStdErr(chunk: string, test?: TestCase, _?: TestResult): void {
    if (!test) {
      for (const line of filteredServerLogLines(chunk)) {
        console.error(`[stderr] ${line}`);
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

    const fsTestTitle = test.title.replaceAll(" ", "-");
    const outputFile = `test-results/debug-pprof-goroutine-${fsTestTitle}.txt`;
    await exportDebugPprof(outputFile);

    if (result.status !== "passed") {
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
    }
    this.testOutput.delete(test.id);
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

const shouldPrintLine = (line: string) =>
  ["  error=EOF", "coderd: audit_log"].every((noise) => !line.includes(noise));

const filteredServerLogLines = (chunk: string): string[] =>
  chunk.trimEnd().split("\n").filter(shouldPrintLine);

const exportDebugPprof = async (outputFile: string) => {
  const response = await axios.get(
    "http://127.0.0.1:6060/debug/pprof/goroutine?debug=1",
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
