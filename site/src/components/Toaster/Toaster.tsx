import { Spinner } from "components/Spinner/Spinner";
import {
	CheckIcon,
	InfoIcon,
	OctagonXIcon,
	TriangleAlertIcon,
	XIcon,
} from "lucide-react";
import { Toaster as Sonner, type ToasterProps as SonnerProps } from "sonner";
import { cn } from "utils/cn";

export const Toaster = ({ ...props }: SonnerProps) => {
	return (
		<Sonner
			icons={{
				success: <CheckIcon className="text-content-success" />,
				info: <InfoIcon className="text-content-primary" />,
				warning: <TriangleAlertIcon className="text-content-warning" />,
				error: <OctagonXIcon className="text-content-destructive" />,
				loading: <Spinner size="sm" loading />,
				close: <XIcon className="size-icon-sm" />,
			}}
			toastOptions={{
				unstyled: true,
				closeButton: true,
				classNames: {
					toast: cn(
						"bg-surface-secondary text-content-secondary border border-solid text-sm p-3 pr-12",
						"shadow rounded-md grid grid-cols-[auto_1fr] w-96 gap-2",
						"data-[expanded=false]:data-[front=false]:overflow-hidden",
						"[&[data-expanded=false][data-front=false]>*]:opacity-0",
					),
					title: "text-content-primary",
					description: "mt-1",
					icon: "pt-1 [&_svg]:size-icon-sm flex flex-col",
					actionButton: cn(
						"border border-solid bg-transparent text-xs rounded-md cursor-pointer",
						"flex items-center gap-2 mt-1 py-1.5 px-2 col-start-2 justify-self-start",
						"[&_svg]:size-icon-xs [&_span]:text-xs text-content-primary",
					),
					closeButton:
						"absolute top-4 right-3 bg-transparent border-none p-0 text-content-primary",
					// Loading styles require a bit more love, the icon doesn't render inline.
					loader: "!left-5 !top-7 !-translate-x-[none]",
					loading: "!pl-[30px]",
				},
			}}
			{...props}
		/>
	);
};
