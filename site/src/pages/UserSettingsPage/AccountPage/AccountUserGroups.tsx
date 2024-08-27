import { useTheme } from "@emotion/react";
import Grid from "@mui/material/Grid";
import { isApiError } from "api/errors";
import type { GroupWithOrganizationInfo } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { AvatarCard } from "components/AvatarCard/AvatarCard";
import { Loader } from "components/Loader/Loader";
import type { FC } from "react";
import { Section } from "../Section";

type AccountGroupsProps = {
	groups: readonly GroupWithOrganizationInfo[] | undefined;
	error: unknown;
	loading: boolean;
};

export const AccountUserGroups: FC<AccountGroupsProps> = ({
	groups,
	error,
	loading,
}) => {
	const theme = useTheme();

	return (
		<Section
			title="Your groups"
			layout="fluid"
			description={
				groups && (
					<span>
						You are in{" "}
						<em
							css={{
								fontStyle: "normal",
								color: theme.palette.text.primary,
								fontWeight: 600,
							}}
						>
							{groups.length} group
							{groups.length !== 1 && "s"}
						</em>
					</span>
				)
			}
		>
			<div css={{ display: "flex", flexFlow: "column nowrap", rowGap: "24px" }}>
				{isApiError(error) && <ErrorAlert error={error} />}

				{groups && (
					<Grid container columns={{ xs: 1, md: 2 }} spacing="16px">
						{groups.map((group) => (
							<Grid item key={group.id} xs={1}>
								<AvatarCard
									background
									imgUrl={group.avatar_url}
									altText={group.display_name || group.name}
									header={group.display_name || group.name}
									subtitle={group.organization_display_name}
								/>
							</Grid>
						))}
					</Grid>
				)}

				{loading && <Loader />}
			</div>
		</Section>
	);
};
