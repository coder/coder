import fs from "fs";
import type {
  FullConfig,
  Suite,
  TestCase,
  TestResult,
  FullResult,
  Reporter,
} from "@playwright/test/reporter";
import axios from "axios";

class CoderReporter implements Reporter {
  onBegin(config: FullConfig, suite: Suite) {
    // eslint-disable-next-line no-console -- Helpful for debugging
    console.log(`Starting the run with ${suite.allTests().length} tests`);
  }

  onTestBegin(test: TestCase) {
    // eslint-disable-next-line no-console -- Helpful for debugging
    console.log(`Starting test ${test.title}`);
  }

  onStdOut(chunk: string, test: TestCase, _: TestResult): void {
    // eslint-disable-next-line no-console -- Helpful for debugging
    console.log(
      `[stdout] [${test ? test.title : "unknown"}]: ${chunk.replace(
        /\n$/g,
        "",
      )}`,
    );
  }

  onStdErr(chunk: string, test: TestCase, _: TestResult): void {
    // eslint-disable-next-line no-console -- Helpful for debugging
    console.log(
      `[stderr] [${test ? test.title : "unknown"}]: ${chunk.replace(
        /\n$/g,
        "",
      )}`,
    );
  }

  async onTestEnd(test: TestCase, result: TestResult) {
    // eslint-disable-next-line no-console -- Helpful for debugging
    console.log(`Finished test ${test.title}: ${result.status}`);

    if (result.status !== "passed") {
      // eslint-disable-next-line no-console -- Helpful for debugging
      console.log("errors", result.errors, "attachments", result.attachments);
    }
    await exportDebugPprof(test.title);
  }

  onEnd(result: FullResult) {
    // eslint-disable-next-line no-console -- Helpful for debugging
    console.log(`Finished the run: ${result.status}`);
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
