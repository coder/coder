import Box, { BoxProps } from "@mui/material/Box";
import CircularProgress from "@mui/material/CircularProgress";
import { FC } from "react";

export const Loader: FC<{ size?: number } & BoxProps> = ({
  size = 26,
  ...boxProps
}) => {
  return (
    <Box
      p={4}
      width="100%"
      display="flex"
      alignItems="center"
      justifyContent="center"
      data-testid="loader"
      {...boxProps}
    >
      <CircularProgress size={size} />
    </Box>
  );
};
