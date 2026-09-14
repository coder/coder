import type { FC, ReactNode } from "react";
import { Margins, type Size } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
export interface FullPageFormProps {
	title: string;
	detail?: ReactNode;
	children?: ReactNode;
	size?: Size;
}

export const FullPageForm: FC<FullPageFormProps> = ({
	title,
	detail,
	children,
	size = "small",
}) => {
	return (
		<Margins size={size}>
			<PageHeader className="pb-6">
				<PageHeaderTitle>{title}</PageHeaderTitle>
				{detail && <PageHeaderSubtitle>{detail}</PageHeaderSubtitle>}
			</PageHeader>

			<div>{children}</div>
		</Margins>
	);
};
