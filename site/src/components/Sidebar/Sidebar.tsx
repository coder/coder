import Box, { BoxProps } from "@mui/material/Box";
import { styled } from "@mui/styles";
import { colors } from "theme/colors";

export const Sidebar = styled((props: BoxProps) => (
  <Box {...props} component="nav" />
))(({ theme }) => ({
  width: theme.spacing(32),
  flexShrink: 0,
  borderRight: `1px solid ${theme.palette.divider}`,
  height: "100%",
  overflowY: "auto",
}));

export const SidebarItem = styled(
  ({ active, ...props }: BoxProps & { active?: boolean }) => (
    <Box component="button" {...props} />
  ),
)(({ theme, active }) => ({
  background: active ? colors.gray[13] : "none",
  border: "none",
  fontSize: 14,
  width: "100%",
  textAlign: "left",
  padding: theme.spacing(0, 3),
  cursor: "pointer",
  pointerEvents: active ? "none" : "auto",
  color: active ? theme.palette.text.primary : theme.palette.text.secondary,
  "&:hover": {
    background: theme.palette.action.hover,
    color: theme.palette.text.primary,
  },
  paddingTop: theme.spacing(1.25),
  paddingBottom: theme.spacing(1.25),
}));

export const SidebarCaption = styled(Box)(({ theme }) => ({
  fontSize: 10,
  textTransform: "uppercase",
  fontWeight: 500,
  color: theme.palette.text.secondary,
  padding: theme.spacing(1.5, 3),
  letterSpacing: "0.5px",
}));
