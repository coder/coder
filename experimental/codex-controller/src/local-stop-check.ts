// Real Docker process-stop test, deliberately no OpenAI session or credentials.

import { execFileSync } from "node:child_process";
import { coderEnv, validateWorkspace } from "./coder.ts";
import {
	DOCKER_IMAGE,
	dockerContainerName,
	dockerLabels,
	stopDockerWorkspaceExecutor,
} from "./docker.ts";

const workspace = validateWorkspace(process.env.WORKSPACE ?? "");
const session = `sess_local_stop_${Date.now()}`;
const args = ["create", "--name", dockerContainerName(session)];
for (const [key, value] of Object.entries(
	dockerLabels(
		workspace,
		session,
		"env_local_stop",
		"https://api.openai.com/v1/agents/api/connect/local",
	),
))
	args.push("--label", `${key}=${value}`);
args.push(
	"--user",
	"1000:1000",
	"--read-only",
	"--cap-drop",
	"ALL",
	"--security-opt",
	"no-new-privileges",
	"--tmpfs",
	"/tmp:rw,nosuid,nodev,size=64m",
	"--mount",
	"type=bind,src=/home/coder/demo,dst=/home/coder/demo",
	"--entrypoint",
	"python3",
	DOCKER_IMAGE,
	"-c",
	'import time;from pathlib import Path;p=Path("/home/coder/demo/local-stop.progress");\nfor i in range(300):\n p.write_text(str(i));time.sleep(1)',
);
function remote(code: string) {
	return execFileSync(
		"coder",
		[
			"ssh",
			"--disable-autostart",
			workspace,
			"--",
			`python3 -c "import base64;exec(base64.b64decode('${Buffer.from(code).toString("base64")}'))"`,
		],
		{ env: coderEnv(), encoding: "utf8", timeout: 30000 },
	).trim();
}
const cid = remote(
	`import subprocess;print(subprocess.check_output(${JSON.stringify(["docker", ...args])},text=True).strip())`,
);
try {
	remote(
		`import subprocess;subprocess.run(['docker','start','${cid}'],check=True)`,
	);
	const before = remote(
		`from pathlib import Path;import time;p=Path('/home/coder/demo/local-stop.progress');end=time.monotonic()+10\nwhile not p.exists() and time.monotonic()<end:time.sleep(.1)\nprint(p.read_text())`,
	);
	await stopDockerWorkspaceExecutor(workspace, session);
	const output = remote(
		`import subprocess,time;from pathlib import Path;p=Path('/home/coder/demo/local-stop.progress');a=p.read_text();time.sleep(2);assert p.read_text()==a;result=subprocess.run(['docker','container','inspect','${cid}'],capture_output=True);assert result.returncode!=0;print('Owned container removed; command progress stopped at '+a)`,
	);
	console.info(
		JSON.stringify({
			test: "real-local-docker-stop",
			passed: true,
			before,
			output,
			openaiUsed: false,
		}),
	);
} finally {
	await stopDockerWorkspaceExecutor(workspace, session);
}
