import { Button } from "components/Button/Button";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import type { FC, ReactNode } from "react";

export interface FullPageHorizontalFormProps {
	title: string;
	detail?: ReactNode;
	onCancel?: () => void;
	children?: ReactNode;
}

export const FullPageHorizontalForm: FC<FullPageHorizontalFormProps> = ({
	title,
	detail,
	onCancel,
	children,
}) => {
	return (
		<Margins size="medium">
			<PageHeader
				actions={
					onCancel && (
						<Button variant="outline" onClick={onCancel}>
							Cancel
						</Button>
					)
				}
			>
				<PageHeaderTitle>{title}</PageHeaderTitle>
				{detail && <PageHeaderSubtitle>{detail}</PageHeaderSubtitle>}
			</PageHeader>

			<main>{children}</main>
		</Margins>
	);
};
