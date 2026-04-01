import { isApiError } from "api/errors";
import type { Group } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { AvatarCard } from "components/Avatar/AvatarCard";
import { Loader } from "components/Loader/Loader";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { Section } from "../Section";

type AccountGroupsProps = {
	groups: readonly Group[] | undefined;
	error: unknown;
	loading: boolean;
};

export const AccountUserGroups: FC<AccountGroupsProps> = ({
	groups,
	error,
	loading,
}) => {
	const { showOrganizations } = useDashboard();

	return (
		<Section
			title="Your groups"
			layout="fluid"
			description={
				groups && (
					<span>
						You are in{" "}
						<em className="not-italic text-content-primary font-semibold">
							{groups.length} group
							{groups.length !== 1 && "s"}
						</em>
					</span>
				)
			}
		>
			<div className="flex flex-col gap-6">
				{isApiError(error) && <ErrorAlert error={error} />}

				{groups && (
					<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
						{groups.map((group) => (
							<AvatarCard
								key={group.id}
								imgUrl={group.avatar_url}
								header={group.display_name || group.name}
								subtitle={
									showOrganizations ? (
										group.organization_display_name
									) : (
										<>
											{group.total_member_count} member
											{group.total_member_count !== 1 && "s"}
										</>
									)
								}
							/>
						))}
					</div>
				)}

				{loading && <Loader />}
			</div>
		</Section>
	);
};
