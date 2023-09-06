import SvgIcon, { SvgIconProps } from "@mui/material/SvgIcon";

export const CloseIcon = (props: SvgIconProps) => (
  <SvgIcon {...props} viewBox="0 0 31 31">
    <path
      d="M29.5 1.5l-28 28M29.5 29.5l-28-28"
      stroke="currentcolor"
      strokeMiterlimit="10"
      strokeLinecap="square"
    />
  </SvgIcon>
);
