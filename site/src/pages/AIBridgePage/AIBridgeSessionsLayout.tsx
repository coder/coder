import type { FC, PropsWithChildren } from "react";
import { Outlet } from "react-router";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { docs } from "#/utils/docs";

const AIBridgeSessionsLayout: FC<PropsWithChildren> = () => {
	return (
		<>
			<SettingsHeader
				actions={
					<SettingsHeaderDocsLink href={docs("/ai-coder/ai-gateway/audit")}>
						Read the docs
					</SettingsHeaderDocsLink>
				}
			>
				<SettingsHeaderTitle>AI Sessions Logs</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Review and audit AI activity, token usage, and prompt history across
					sessions.
				</SettingsHeaderDescription>
			</SettingsHeader>
			<Outlet />
		</>
	);
};

export default AIBridgeSessionsLayout;
