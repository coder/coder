import { Button } from "#/components/Button/Button";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";

type FullPageHorizontalFormProps = {
	title: string;
	detail?: React.ReactNode;
	onCancel?: () => void;
	children?: React.ReactNode;
};

export const FullPageHorizontalForm: React.FC<FullPageHorizontalFormProps> = ({
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

			<div>{children}</div>
		</Margins>
	);
};
