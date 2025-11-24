import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import type { FC, ReactNode } from "react";
export interface FullPageFormProps {
	title: string;
	detail?: ReactNode;
	children?: ReactNode;
}

export const FullPageForm: FC<FullPageFormProps> = ({
	title,
	detail,
	children,
}) => {
	return (
		<Margins size="small">
			<PageHeader className="pb-6">
				<PageHeaderTitle>{title}</PageHeaderTitle>
				{detail && <PageHeaderSubtitle>{detail}</PageHeaderSubtitle>}
			</PageHeader>

			<main>{children}</main>
		</Margins>
	);
};
