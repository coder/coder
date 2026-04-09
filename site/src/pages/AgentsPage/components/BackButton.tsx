import { ChevronLeftIcon } from "lucide-react";
import type { FC } from "react";

interface BackButtonProps {
	onClick: () => void;
}

export const BackButton: FC<BackButtonProps> = ({ onClick }) => (
	<button
		type="button"
		onClick={onClick}
		className="mb-4 inline-flex cursor-pointer items-center gap-0.5 border-0 bg-transparent p-0 text-sm text-content-secondary transition-colors hover:text-content-primary"
	>
		<ChevronLeftIcon className="h-4 w-4" />
		Back
	</button>
);
