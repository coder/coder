import Collapse from "@mui/material/Collapse";
import Link from "@mui/material/Link";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import type { FC, ReactNode } from "react";
import { cn } from "utils/cn";

interface ExpanderProps {
	expanded: boolean;
	setExpanded: (val: boolean) => void;
	children?: ReactNode;
}

export const Expander: FC<ExpanderProps> = ({
	expanded,
	setExpanded,
	children,
}) => {
	const toggleExpanded = () => setExpanded(!expanded);

	return (
		<>
			{!expanded && (
				<Link onClick={toggleExpanded} className={classNames.expandLink}>
					<span className={classNames.text}>
						Click here to learn more
						<DropdownArrow margin={false} />
					</span>
				</Link>
			)}
			<Collapse in={expanded}>
				<div className={classNames.text}>{children}</div>
			</Collapse>
			{expanded && (
				<Link
					onClick={toggleExpanded}
					className={cn([classNames.expandLink, classNames.collapseLink])}
				>
					<span className={classNames.text}>
						Click here to hide
						<DropdownArrow margin={false} close />
					</span>
				</Link>
			)}
		</>
	);
};

const classNames = {
	expandLink: "cursor-pointer text-content-secondary",
	collapseLink: "mt-4",
	text: "flex items-center text-content-secondary text-xs leading-loose",
};
