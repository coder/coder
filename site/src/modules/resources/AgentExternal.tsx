import { workspaceAgentCredentials } from "api/queries/workspaces";
import type { Workspace, WorkspaceAgent } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { CodeExample } from "components/CodeExample/CodeExample";
import { Loader } from "components/Loader/Loader";
import { type FC, useState } from "react";
import { useQuery } from "react-query";

interface AgentExternalProps {
	agent: WorkspaceAgent;
	workspace: Workspace;
}

type OS = "linux" | "darwin" | "windows";
type Arch = "amd64" | "arm64" | "armv7";

const osByLabel: Record<OS, string> = {
	linux: "Linux",
	darwin: "macOS",
	windows: "Windows",
};

const archsByOS: Record<OS, Arch[]> = {
	linux: ["amd64", "arm64", "armv7"],
	darwin: ["amd64", "arm64"],
	windows: ["amd64", "arm64"],
};

const validOSes = Object.keys(osByLabel) as OS[];

function toValidOS(value: string | undefined): OS {
	if (value && (validOSes as string[]).includes(value)) {
		return value as OS;
	}
	return "linux";
}

function toValidArch(os: OS, value: string | undefined): Arch {
	const archs = archsByOS[os];
	if (value && (archs as string[]).includes(value)) {
		return value as Arch;
	}
	return archs[0];
}

function buildCommand(
	baseURL: string,
	token: string,
	os: OS,
	arch: Arch,
): string {
	const initScriptURL = `${baseURL}/${os}/${arch}`;
	if (os === "windows") {
		return `$env:CODER_AGENT_TOKEN="${token}"; iwr -useb "${initScriptURL}" | iex`;
	}
	return `curl -fsSL "${initScriptURL}" | CODER_AGENT_TOKEN="${token}" sh`;
}

export const AgentExternal: FC<AgentExternalProps> = ({ agent, workspace }) => {
	const {
		data: credentials,
		error,
		isLoading,
		isError,
	} = useQuery(workspaceAgentCredentials(workspace.id, agent.name));

	const defaultOS = toValidOS(agent.operating_system);
	const [selectedOS, setSelectedOS] = useState<OS>(defaultOS);
	const [selectedArch, setSelectedArch] = useState<Arch>(
		toValidArch(defaultOS, agent.architecture),
	);

	if (isLoading) {
		return <Loader />;
	}

	if (isError) {
		return <ErrorAlert error={error} />;
	}

	const availableArchs = archsByOS[selectedOS];
	const arch = toValidArch(selectedOS, selectedArch);

	const command =
		credentials?.init_script_base_url
			? buildCommand(
					credentials.init_script_base_url,
					credentials.agent_token,
					selectedOS,
					arch,
				)
			: (credentials?.command ?? "");

	const handleOSChange = (os: OS) => {
		setSelectedOS(os);
		setSelectedArch(toValidArch(os, selectedArch));
	};

	return (
		<section className="text-base text-muted-foreground pb-2 leading-relaxed flex flex-col gap-3">
			<p>
				Please run the following command to attach an agent to the{" "}
				{workspace.name} workspace:
			</p>
			<div className="flex flex-col gap-2" role="group" aria-label="Platform">
				<div className="flex items-center gap-1.5">
					<span
						id="os-label"
						className="text-xs text-content-secondary font-medium w-16"
					>
						OS
					</span>
					<div className="flex gap-1" role="group" aria-labelledby="os-label">
						{validOSes.map((os) => (
							<Button
								key={os}
								size="sm"
								variant={selectedOS === os ? "default" : "outline"}
								onClick={() => handleOSChange(os)}
								aria-pressed={selectedOS === os}
							>
								{osByLabel[os]}
							</Button>
						))}
					</div>
				</div>
				<div className="flex items-center gap-1.5">
					<span
						id="arch-label"
						className="text-xs text-content-secondary font-medium w-16"
					>
						Arch
					</span>
					<div
						className="flex gap-1"
						role="group"
						aria-labelledby="arch-label"
					>
						{availableArchs.map((a) => (
							<Button
								key={a}
								size="sm"
								variant={arch === a ? "default" : "outline"}
								onClick={() => setSelectedArch(a)}
								aria-pressed={arch === a}
							>
								{a}
							</Button>
						))}
					</div>
				</div>
			</div>
			<CodeExample
				code={command}
				secret={false}
				redactPattern={/CODER_AGENT_TOKEN="([^"]+)"/g}
				redactReplacement={`CODER_AGENT_TOKEN="********"`}
				showRevealButton={true}
			/>
		</section>
	);
};
