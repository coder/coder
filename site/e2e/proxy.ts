import { spawn, type ChildProcess, exec } from "child_process";
import { coderMain, coderPort, workspaceProxyPort } from "./constants";
import { waitUntilUrlIsNotResponding } from "./helpers";

export const startWorkspaceProxy = async (
  token: string,
): Promise<ChildProcess> => {
  const cp = spawn("go", ["run", coderMain, "wsproxy", "server"], {
    env: {
      ...process.env,
      CODER_PRIMARY_ACCESS_URL: `http://127.0.0.1:${coderPort}`,
      CODER_PROXY_SESSION_TOKEN: token,
      CODER_HTTP_ADDRESS: `localhost:${workspaceProxyPort}`,
    },
  });
  cp.stdout.on("data", (data: Buffer) => {
    // eslint-disable-next-line no-console -- Log wsproxy activity
    console.log(
      `[wsproxy] [stdout] [onData] ${data.toString().replace(/\n$/g, "")}`,
    );
  });
  cp.stderr.on("data", (data: Buffer) => {
    // eslint-disable-next-line no-console -- Log wsproxy activity
    console.log(
      `[wsproxy] [stderr] [onData] ${data.toString().replace(/\n$/g, "")}`,
    );
  });
  return cp;
};

export const stopWorkspaceProxy = async (
  cp: ChildProcess,
  goRun: boolean = true,
) => {
  exec(goRun ? `pkill -P ${cp.pid}` : `kill ${cp.pid}`, (error) => {
    if (error) {
      throw new Error(`exec error: ${JSON.stringify(error)}`);
    }
  });
  await waitUntilUrlIsNotResponding(`http://127.0.0.1:${workspaceProxyPort}`);
};
