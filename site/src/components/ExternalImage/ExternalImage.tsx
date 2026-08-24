import { useAppearance } from "#/theme/appearance";
import { getExternalImageStylesFromUrl } from "#/theme/externalImages";

export const ExternalImage: React.FC<React.ComponentPropsWithRef<"img">> = ({
	style,
	alt = "",
	...props
}) => {
	const { externalImages } = useAppearance();

	return (
		<img
			alt={alt}
			style={{
				...getExternalImageStylesFromUrl(externalImages, props.src),
				...style,
			}}
			{...props}
		/>
	);
};
