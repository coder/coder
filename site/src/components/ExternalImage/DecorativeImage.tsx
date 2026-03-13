import { ExternalImage, type ExternalImageProps } from "./ExternalImage";

type DecorativeImageProps = Omit<ExternalImageProps, "alt">;

export const DecorativeImage: React.FC<DecorativeImageProps> = (props) => {
	return <ExternalImage {...props} alt="" aria-hidden="true" />;
};
