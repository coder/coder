import { css } from "@emotion/react";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type { DeploymentValues, ExternalAuthConfig } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { PremiumBadge } from "components/Badges/Badges";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import type { FC } from "react";
import { docs } from "utils/docs";

export type ExternalAuthSettingsPageViewProps = {
	config: DeploymentValues;
};

export const ExternalAuthSettingsPageView: FC<
	ExternalAuthSettingsPageViewProps
> = ({ config }) => {
	return (
		<>
			<SettingsHeader
				actions={<SettingsHeaderDocsLink href={docs("/admin/external-auth")} />}
			>
				<SettingsHeaderTitle>External Authentication</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Coder integrates with GitHub, GitLab, BitBucket, Azure Repos, and
					OpenID Connect to authenticate developers with external services.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<video
				autoPlay
				muted
				loop
				playsInline
				src="/external-auth.mp4"
				style={{
					maxWidth: "100%",
					borderRadius: 4,
				}}
			/>

			<div
				css={{
					marginTop: 24,
					marginBottom: 24,
				}}
			>
				<Alert severity="info" actions={<PremiumBadge key="enterprise" />}>
					Integrating with multiple External authentication providers is an
					Premium feature.
				</Alert>
			</div>

			<TableContainer>
				<Table
					css={css`
            & td {
              padding-top: 24px;
              padding-bottom: 24px;
            }

            & td:last-child,
            & th:last-child {
              padding-left: 32px;
            }
          `}
				>
					<TableHead>
						<TableRow>
							<TableCell width="25%">ID</TableCell>
							<TableCell width="25%">Client ID</TableCell>
							<TableCell width="25%">Match</TableCell>
						</TableRow>
					</TableHead>
					<TableBody>
						{((config.external_auth === null ||
							config.external_auth?.length === 0) && (
							<TableRow>
								<TableCell colSpan={999}>
									<div css={{ textAlign: "center" }}>
										No providers have been configured!
									</div>
								</TableCell>
							</TableRow>
						)) ||
							config.external_auth?.map((git: ExternalAuthConfig) => {
								const name = git.id || git.type;
								return (
									<TableRow key={name}>
										<TableCell>{name}</TableCell>
										<TableCell>{git.client_id}</TableCell>
										<TableCell>{git.regex || "Not Set"}</TableCell>
									</TableRow>
								);
							})}
					</TableBody>
				</Table>
			</TableContainer>
		</>
	);
};
