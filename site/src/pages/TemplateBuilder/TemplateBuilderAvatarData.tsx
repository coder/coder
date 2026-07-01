import type { FC, PropsWithChildren } from "react";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Link } from "#/components/Link/Link";

type TemplateBuilderAvatarDataProps = PropsWithChildren<{
	name: string;
	description: string;
	iconUrl?: string;
	detailsUrl?: string;
}>;

export const TemplateBuilderAvatarData: FC<TemplateBuilderAvatarDataProps> = ({
	name,
	description,
	iconUrl,
	detailsUrl,
}) => {
	return (
		<AvatarData
			avatar={<Avatar src={iconUrl} size="lg" variant="icon" />}
			title={<h3 className="m-0 text-xl font-semibold">{name}</h3>}
			subtitle={
				<>
					<p className="text-xs font-normal text-content-secondary inline">
						{description}
					</p>
					{detailsUrl && (
						<Link
							href={detailsUrl}
							target="_blank"
							size="sm"
							className="text-xs font-normal ml-1"
						>
							View details
						</Link>
					)}
				</>
			}
		/>
	);
};
