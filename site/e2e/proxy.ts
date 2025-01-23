import { type ChildProcess, exec, spawn } from "node:child_process";
import { coderBinary, coderPort, workspaceProxyPort } from "./constants";
import { waitUntilUrlIsNotResponding } from "./helpers";

export const startWorkspaceProxy = async (
	token: string,
): Promise<ChildProcess> => {
	const cp = spawn(coderBinary, ["wsproxy", "server"], {
		env: {
			...process.env,
			CODER_PRIMARY_ACCESS_URL: `http://127.0.0.1:${coderPort}`,
			CODER_PROXY_SESSION_TOKEN: token,
			CODER_HTTP_ADDRESS: `localhost:${workspaceProxyPort}`,
		},
	});
	cp.stdout.on("data", (data: Buffer) => {
		console.info(
			`[wsproxy] [stdout] [onData] ${data.toString().replace(/\n$/g, "")}`,
		);
	});
	cp.stderr.on("data", (data: Buffer) => {
		console.info(
			`[wsproxy] [stderr] [onData] ${data.toString().replace(/\n$/g, "")}`,
		);
	});
	return cp;
};

export const stopWorkspaceProxy = async (cp: ChildProcess) => {
	exec(`kill ${cp.pid}`, (error) => {
		if (error) {
			throw new Error(`exec error: ${JSON.stringify(error)}`);
		}
	});
	await waitUntilUrlIsNotResponding(`http://127.0.0.1:${workspaceProxyPort}`);
};
