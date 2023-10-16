import * as fs from "fs";
import type {
  FullConfig,
  Suite,
  TestCase,
  TestResult,
  FullResult,
  Reporter,
} from "@playwright/test/reporter";
import axios from "axios";
import type { Writable } from "stream";

const testOutput = new Map<string, Array<[Writable, string]>>();

class CoderReporter implements Reporter {
  onBegin(config: FullConfig, suite: Suite) {
    // eslint-disable-next-line no-console -- Helpful for debugging
    console.log(`==> Running ${suite.allTests().length} tests`);
  }

  onTestBegin(test: TestCase) {
    testOutput.set(test.id, []);
    // eslint-disable-next-line no-console -- Helpful for debugging
    console.log(`==> Starting test ${test.title}`);
  }

  onStdOut(chunk: string, test?: TestCase, _?: TestResult): void {
    if (!test) {
      // console.log(`[stdout] [unknown] ${chunk.replace(/\n$/g, "")}`);
      return;
    }
    testOutput.get(test.id)!.push([process.stdout, chunk]);
  }

  onStdErr(chunk: string, test?: TestCase, _?: TestResult): void {
    if (!test) {
      // console.error(`[stderr] [unknown] ${chunk.replace(/\n$/g, "")}`);
      return;
    }
    testOutput.get(test.id)!.push([process.stderr, chunk]);
  }

  async onTestEnd(test: TestCase, result: TestResult) {
    // eslint-disable-next-line no-console -- Helpful for debugging
    console.log(`==> Finished test ${test.title}: ${result.status}`);

    if (result.status !== "passed") {
      // eslint-disable-next-line no-console -- Debugging output
      console.log("==> Output");
      const output = testOutput.get(test.id)!;
      for (const [target, chunk] of output) {
        target.write(`${chunk.replace(/\n$/g, "")}\n`);
      }

      if (result.errors.length > 0) {
        // eslint-disable-next-line no-console -- Debugging output
        console.log("==> Errors");
        for (const error of result.errors) {
          if (error.location) {
            // eslint-disable-next-line no-console -- Debugging output
            console.log(`${error.location.file}:${error.location.line}:`);
          }
          if (error.snippet) {
            // eslint-disable-next-line no-console -- Debugging output
            console.log(error.snippet);
          }

          if (error.message) {
            // eslint-disable-next-line no-console -- Debugging output
            console.log(error.message);
          } else {
            // eslint-disable-next-line no-console -- Debugging output
            console.log(error);
          }
        }
      }

      if (result.attachments.length > 0) {
        // eslint-disable-next-line no-console -- Debugging output
        console.log("==> Attachments");
        for (const attachment of result.attachments) {
          // eslint-disable-next-line no-console -- Debugging output
          console.log(attachment);
        }
      }
    }
    testOutput.delete(test.id);
    await exportDebugPprof(test.title);
  }

  onEnd(result: FullResult) {
    // eslint-disable-next-line no-console -- Helpful for debugging
    console.log(`==> Tests ${result.status}`);
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
          // eslint-disable-next-line no-console -- Helpful for debugging
          console.log(`Data from ${url} has been saved to ${outputFile}`);
        }
      });
    })
    .catch((error) => {
      throw new Error(`Error: ${error.message}`);
    });
};

// eslint-disable-next-line no-unused-vars -- Playwright config uses it
export default CoderReporter;
