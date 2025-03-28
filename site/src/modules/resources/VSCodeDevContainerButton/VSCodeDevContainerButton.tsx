import KeyboardArrowDownIcon from "@mui/icons-material/KeyboardArrowDown";
import ButtonGroup from "@mui/material/ButtonGroup";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import { API } from "api/api";
import type { DisplayApp } from "api/typesGenerated";
import { VSCodeIcon } from "components/Icons/VSCodeIcon";
import { VSCodeInsidersIcon } from "components/Icons/VSCodeInsidersIcon";
import { type FC, useRef, useState } from "react";
import { AgentButton } from "../AgentButton";
import { DisplayAppNameMap } from "../AppLink/AppLink";

export interface VSCodeDevContainerButtonProps {
	userName: string;
	workspaceName: string;
	agentName?: string;
	folderPath: string;
	devContainerPath: string;
	devContainerFolder: string;
	displayApps: readonly DisplayApp[];
}

type VSCodeVariant = "vscode" | "vscode-insiders";

const VARIANT_KEY = "vscode-variant";

export const VSCodeDevContainerButton: FC<VSCodeDevContainerButtonProps> = (
	props,
) => {
	const [isVariantMenuOpen, setIsVariantMenuOpen] = useState(false);
	const previousVariant = localStorage.getItem(VARIANT_KEY);
	const [variant, setVariant] = useState<VSCodeVariant>(() => {
		if (!previousVariant) {
			return "vscode";
		}
		return previousVariant as VSCodeVariant;
	});
	const menuAnchorRef = useRef<HTMLDivElement>(null);

	const selectVariant = (variant: VSCodeVariant) => {
		localStorage.setItem(VARIANT_KEY, variant);
		setVariant(variant);
		setIsVariantMenuOpen(false);
	};

	const includesVSCodeDesktop = props.displayApps.includes("vscode");
	const includesVSCodeInsiders = props.displayApps.includes("vscode_insiders");

	return includesVSCodeDesktop && includesVSCodeInsiders ? (
		<div>
			<ButtonGroup ref={menuAnchorRef} variant="outlined">
				{variant === "vscode" ? (
					<VSCodeButton {...props} />
				) : (
					<VSCodeInsidersButton {...props} />
				)}

				<AgentButton
					aria-controls={
						isVariantMenuOpen ? "vscode-variant-button-menu" : undefined
					}
					aria-expanded={isVariantMenuOpen ? "true" : undefined}
					aria-label="select VSCode variant"
					aria-haspopup="menu"
					disableRipple
					onClick={() => {
						setIsVariantMenuOpen(true);
					}}
					css={{ paddingLeft: 0, paddingRight: 0 }}
				>
					<KeyboardArrowDownIcon css={{ fontSize: 16 }} />
				</AgentButton>
			</ButtonGroup>

			<Menu
				open={isVariantMenuOpen}
				anchorEl={menuAnchorRef.current}
				onClose={() => setIsVariantMenuOpen(false)}
				css={{
					"& .MuiMenu-paper": {
						width: menuAnchorRef.current?.clientWidth,
					},
				}}
			>
				<MenuItem
					css={{ fontSize: 14 }}
					onClick={() => {
						selectVariant("vscode");
					}}
				>
					<VSCodeIcon css={{ width: 12, height: 12 }} />
					{DisplayAppNameMap.vscode}
				</MenuItem>
				<MenuItem
					css={{ fontSize: 14 }}
					onClick={() => {
						selectVariant("vscode-insiders");
					}}
				>
					<VSCodeInsidersIcon css={{ width: 12, height: 12 }} />
					{DisplayAppNameMap.vscode_insiders}
				</MenuItem>
			</Menu>
		</div>
	) : includesVSCodeDesktop ? (
		<VSCodeButton {...props} />
	) : (
		<VSCodeInsidersButton {...props} />
	);
};

const VSCodeButton: FC<VSCodeDevContainerButtonProps> = ({
	userName,
	workspaceName,
	agentName,
	folderPath,
	devContainerPath,
	devContainerFolder,
}) => {
	const [loading, setLoading] = useState(false);

	return (
		<AgentButton
			startIcon={<VSCodeIcon />}
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
							devContainerPath,
							devContainerFolder,
						});
						if (agentName) {
							query.set("agent", agentName);
						}
						if (folderPath) {
							query.set("folder", folderPath);
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
			{DisplayAppNameMap.vscode}
		</AgentButton>
	);
};

const VSCodeInsidersButton: FC<VSCodeDevContainerButtonProps> = ({
	userName,
	workspaceName,
	agentName,
	folderPath,
	devContainerPath,
	devContainerFolder,
}) => {
	const [loading, setLoading] = useState(false);

	return (
		<AgentButton
			startIcon={<VSCodeInsidersIcon />}
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
							devContainerPath,
							devContainerFolder,
						});
						if (agentName) {
							query.set("agent", agentName);
						}
						if (folderPath) {
							query.set("folder", folderPath);
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
			{DisplayAppNameMap.vscode_insiders}
		</AgentButton>
	);
};
