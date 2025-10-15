import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import type { FC, PropsWithChildren } from "react";
import { Outlet } from "react-router";

const Language = {
	title: "AI Governance",
	subtitle: "Manage usage for your organization.",
};

const AIGovernanceLayout: FC<PropsWithChildren> = ({
	children = <Outlet />,
}) => {
	return (
		<Margins className="pb-12">
			<PageHeader>
				<PageHeaderTitle>
					<Stack direction="row" spacing={1} alignItems="center">
						<span>{Language.title}</span>
					</Stack>
				</PageHeaderTitle>
				<PageHeaderSubtitle>{Language.subtitle}</PageHeaderSubtitle>
			</PageHeader>
			{children}
		</Margins>
	);
};

export default AIGovernanceLayout;
