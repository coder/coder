import CloseIcon from "@mui/icons-material/Close";
import Sell from "@mui/icons-material/Sell";
import IconButton from "@mui/material/IconButton";
import type { FC } from "react";

// TODO: Importing from a page in here sucks, but idk how to refactor this...
// it's kind of a mess of a file...
import { BooleanPill, Pill } from "pages/HealthPage/Content";

const parseBool = (s: string): { valid: boolean; value: boolean } => {
	switch (s.toLowerCase()) {
		case "true":
		case "yes":
		case "1":
			return { valid: true, value: true };
		case "false":
		case "no":
		case "0":
		case "":
			return { valid: true, value: false };
		default:
			return { valid: false, value: false };
	}
};

interface ProvisionerTagProps {
	tagName: string;
	tagValue: string;
	/** Only used in the TemplateVersionEditor */
	onDelete?: (tagName: string) => void;
}

export const ProvisionerTag: FC<ProvisionerTagProps> = ({
	tagName,
	tagValue,
	onDelete,
}) => {
	const { valid, value: boolValue } = parseBool(tagValue);
	const kv = `${tagName}: ${tagValue}`;
	const content = onDelete ? (
		<>
			{kv}
			<IconButton
				aria-label={`delete-${tagName}`}
				size="small"
				color="secondary"
				onClick={() => {
					onDelete(tagName);
				}}
			>
				<CloseIcon fontSize="inherit" css={{ width: 14, height: 14 }} />
			</IconButton>
		</>
	) : (
		kv
	);
	if (valid) {
		return <BooleanPill value={boolValue}>{content}</BooleanPill>;
	}
	return <Pill icon={<Sell />}>{content}</Pill>;
};
