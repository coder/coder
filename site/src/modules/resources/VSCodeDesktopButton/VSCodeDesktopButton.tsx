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
import { VSCodeIcon } from "#/components/Icons/VSCodeIcon";
import { VSCodeInsidersIcon } from "#/components/Icons/VSCodeInsidersIcon";
import { useStorage } from "#/hooks/useStorage";
import { getVSCodeHref } from "#/modules/apps/apps";
import { defineStorageKey, stringLiteralCodec } from "#/storage";
import { AgentButton } from "../AgentButton";
import { DisplayAppNameMap } from "../AppLink/AppLink";

type VSCodeVariant = "vscode" | "vscode-insiders";

/** Shared with VSCodeDevContainerButton so both buttons track the same choice. */
export const vscodeVariantStorage = defineStorageKey<VSCodeVariant>({
	key: "vscode-variant",
	codec: stringLiteralCodec<VSCodeVariant>({
		oneOf: ["vscode", "vscode-insiders"],
	}),
	defaultValue: "vscode",
});

interface VSCodeDesktopButtonProps {
	userName: string;
	workspaceName: string;
	agentName?: string;
	folderPath?: string;
	displayApps: readonly DisplayApp[];
}

export const VSCodeDesktopButton: FC<VSCodeDesktopButtonProps> = (props) => {
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
						<VSCodeIcon className="size-3" />
						{DisplayAppNameMap.vscode}
					</DropdownMenuItem>
					<DropdownMenuItem
						onClick={() => {
							setVariant("vscode-insiders");
						}}
					>
						<VSCodeInsidersIcon className="size-3" />
						{DisplayAppNameMap.vscode_insiders}
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
		</div>
	) : includesVSCodeDesktop ? (
		<VSCodeButton {...props} />
	) : (
		<VSCodeInsidersButton {...props} />
	);
};

const VSCodeButton: FC<VSCodeDesktopButtonProps> = ({
	userName,
	workspaceName,
	agentName,
	folderPath,
}) => {
	const [loading, setLoading] = useState(false);

	return (
		<AgentButton
			disabled={loading}
			onClick={() => {
				setLoading(true);
				API.getApiKey()
					.then(({ key }) => {
						location.href = getVSCodeHref("vscode", {
							owner: userName,
							workspace: workspaceName,
							token: key,
							agent: agentName,
							folder: folderPath,
						});
					})
					.catch((ex) => {
						console.error(ex);
					})
					.finally(() => {
						setLoading(false);
					});
			}}
		>
			<VSCodeIcon />
			{DisplayAppNameMap.vscode}
		</AgentButton>
	);
};

const VSCodeInsidersButton: FC<VSCodeDesktopButtonProps> = ({
	userName,
	workspaceName,
	agentName,
	folderPath,
}) => {
	const [loading, setLoading] = useState(false);

	return (
		<AgentButton
			disabled={loading}
			onClick={() => {
				setLoading(true);
				API.getApiKey()
					.then(({ key }) => {
						location.href = getVSCodeHref("vscode-insiders", {
							owner: userName,
							workspace: workspaceName,
							token: key,
							agent: agentName,
							folder: folderPath,
						});
					})
					.catch((ex) => {
						console.error(ex);
					})
					.finally(() => {
						setLoading(false);
					});
			}}
		>
			<VSCodeInsidersIcon />
			{DisplayAppNameMap.vscode_insiders}
		</AgentButton>
	);
};
