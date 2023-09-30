import { makeStyles } from "@mui/styles";
import { FC } from "react";
import { combineClasses } from "utils/combineClasses";
import {
  containerWidth,
  containerWidthMedium,
  sidePadding,
} from "theme/constants";

type Size = "regular" | "medium" | "small";

const widthBySize: Record<Size, number> = {
  regular: containerWidth,
  medium: containerWidthMedium,
  small: containerWidth / 3,
};

const useStyles = makeStyles(() => ({
  margins: {
    margin: "0 auto",
    maxWidth: ({ maxWidth }: { maxWidth: number }) => maxWidth,
    padding: `0 ${sidePadding}px`,
    width: "100%",
  },
}));

export const Margins: FC<JSX.IntrinsicElements["div"] & { size?: Size }> = ({
  size = "regular",
  ...divProps
}) => {
  const styles = useStyles({ maxWidth: widthBySize[size] });
  return (
    <div
      {...divProps}
      className={combineClasses([styles.margins, divProps.className])}
    />
  );
};
