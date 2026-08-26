import { type FC, useId, useRef, useState } from "react";
import { API } from "#/api/api";
import type { DisplayApp } from "#/api/typesGenerated";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { useStorage } from "#/hooks/useStorage";
import { AgentButton } from "../AgentButton";
import { DisplayAppNameMap } from "../AppLink/AppLink";
import { vscodeVariantStorage } from "../VSCodeDesktopButton/VSCodeDesktopButton";

interface VSCodeDevContainerButtonProps {
	userName: string;
	workspaceName: string;
	agentName?: string;
	devContainerName: string;
	devContainerFolder: string;
	localWorkspaceFolder: string;
	localConfigFile: string;
	displayApps: readonly DisplayApp[];
}

export const VSCodeDevContainerButton: FC<VSCodeDevContainerButtonProps> = (
	props,
) => {
	const [isVariantMenuOpen, setIsVariantMenuOpen] = useState(false);
	const [variant, setVariant] = useStorage(vscodeVariantStorage);
	const menuAnchorRef = useRef<HTMLDivElement>(null);
	const menuContentId = useId();

	const includesVSCodeDesktop = props.displayApps.includes("vscode");
	const includesVSCodeInsiders = props.displayApps.includes("vscode_insiders");

	return includesVSCodeDesktop && includesVSCodeInsiders ? (
		<div ref={menuAnchorRef} className="inline-flex items-center gap-1">
			{variant === "vscode" ? (
				<VSCodeButton {...props} />
			) : (
				<VSCodeInsidersButton {...props} />
			)}

			<DropdownMenu
				open={isVariantMenuOpen}
				onOpenChange={setIsVariantMenuOpen}
			>
				<DropdownMenuTrigger asChild>
					<AgentButton
						aria-controls={isVariantMenuOpen ? menuContentId : undefined}
						aria-label="select VSCode variant"
						size="icon-lg"
					>
						<ChevronDownIcon open={isVariantMenuOpen} />
					</AgentButton>
				</DropdownMenuTrigger>

				<DropdownMenuContent
					id={menuContentId}
					align="end"
					collisionPadding={16}
					style={{ width: menuAnchorRef.current?.clientWidth }}
				>
					<DropdownMenuItem
						onClick={() => {
							setVariant("vscode");
						}}
					>
						<ExternalImage src="/icon/code.svg" alt="" className="size-3" />
						{DisplayAppNameMap.vscode}
					</DropdownMenuItem>
					<DropdownMenuItem
						onClick={() => {
							setVariant("vscode-insiders");
						}}
					>
						<ExternalImage
							src="/icon/code-insiders.svg"
							alt=""
							className="size-3"
						/>
						{DisplayAppNameMap.vscode_insiders}
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
		</div>
	) : includesVSCodeDesktop ? (
		<VSCodeButton {...props} />
	) : includesVSCodeInsiders ? (
		<VSCodeInsidersButton {...props} />
	) : null;
};

const VSCodeButton: FC<VSCodeDevContainerButtonProps> = ({
	userName,
	workspaceName,
	agentName,
	devContainerName,
	devContainerFolder,
	localWorkspaceFolder,
	localConfigFile,
}) => {
	const [loading, setLoading] = useState(false);

	return (
		<AgentButton
			disabled={loading}
			onClick={() => {
				setLoading(true);
				API.getApiKey()
					.then(({ key }) => {
						const query = new URLSearchParams({
							owner: userName,
							workspace: workspaceName,
							url: location.origin,
							token: key,
							devContainerName,
							devContainerFolder,
							localWorkspaceFolder,
							localConfigFile,
						});
						if (agentName) {
							query.set("agent", agentName);
						}

						location.href = `vscode://coder.coder-remote/openDevContainer?${query.toString()}`;
					})
					.catch((ex) => {
						console.error(ex);
					})
					.finally(() => {
						setLoading(false);
					});
			}}
		>
			<ExternalImage src="/icon/code.svg" alt="" />
			{DisplayAppNameMap.vscode}
		</AgentButton>
	);
};

const VSCodeInsidersButton: FC<VSCodeDevContainerButtonProps> = ({
	userName,
	workspaceName,
	agentName,
	devContainerName,
	devContainerFolder,
	localWorkspaceFolder,
	localConfigFile,
}) => {
	const [loading, setLoading] = useState(false);

	return (
		<AgentButton
			disabled={loading}
			onClick={() => {
				setLoading(true);
				API.getApiKey()
					.then(({ key }) => {
						const query = new URLSearchParams({
							owner: userName,
							workspace: workspaceName,
							url: location.origin,
							token: key,
							devContainerName,
							devContainerFolder,
							localWorkspaceFolder,
							localConfigFile,
						});
						if (agentName) {
							query.set("agent", agentName);
						}

						location.href = `vscode-insiders://coder.coder-remote/openDevContainer?${query.toString()}`;
					})
					.catch((ex) => {
						console.error(ex);
					})
					.finally(() => {
						setLoading(false);
					});
			}}
		>
			<ExternalImage src="/icon/code-insiders.svg" alt="" />
			{DisplayAppNameMap.vscode_insiders}
		</AgentButton>
	);
};
