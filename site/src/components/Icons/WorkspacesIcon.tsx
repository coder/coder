import SvgIcon, { SvgIconProps } from "@material-ui/core/SvgIcon"

export const WorkspacesIcon: typeof SvgIcon = (props: SvgIconProps) => (
  <SvgIcon {...props} viewBox="0 0 16 16">
    <path d="M6 14H2V2H12V5.5L14 7V1C14 0.734784 13.8946 0.48043 13.7071 0.292893C13.5196 0.105357 13.2652 0 13 0L1 0C0.734784 0 0.48043 0.105357 0.292893 0.292893C0.105357 0.48043 0 0.734784 0 1L0 15C0 15.2652 0.105357 15.5196 0.292893 15.7071C0.48043 15.8946 0.734784 16 1 16H6V14Z" />
    <path d="M12 8L8 11V16H11V13H13.035V16H16V11L12 8Z" />
    <path d="M10 4H4V5H10V4Z" />
    <path d="M10 7H4V8H10V7Z" />
    <path d="M7 10H4V11H7V10Z" />
  </SvgIcon>
)
